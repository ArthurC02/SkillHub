package main

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestTheRealRepositoryHasNoDomainVocabularyDrift(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := domainVocabularyProblems(root); len(problems) > 0 {
		t.Fatalf("domain vocabularies drifted:\n%s", strings.Join(problems, "\n"))
	}

	if len(domainVocabularies) < 2 {
		t.Fatalf("only %d vocabulary is reconciled; a table this short cannot be the reason the check passed", len(domainVocabularies))
	}
	for _, vocabulary := range domainVocabularies {
		if len(vocabulary.sources) < 2 {
			t.Errorf("%s is read from %d place; reconciliation needs at least two", vocabulary.name, len(vocabulary.sources))
		}
		for _, source := range vocabulary.sources {
			values, err := source.read(root)
			if err != nil {
				t.Errorf("%s: %s: %v", vocabulary.name, source.label, err)
				continue
			}
			if len(values) == 0 {
				t.Errorf("%s: %s yielded nothing, so the comparison above compared nothing", vocabulary.name, source.label)
			}
		}
		for _, reader := range vocabulary.readers {
			if values, err := reader.read(root); err != nil || len(values) == 0 {
				t.Errorf("%s: reader %s yielded %v (%v), so it was checked against nothing", vocabulary.name, reader.label, values, err)
			}
		}
	}

	statuses, err := postgresEnumValues(
		filepath.Join(root, filepath.FromSlash("db/migrations/0004_test_lab_and_runs.sql")), "run_status")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"queued", "running", "succeeded", "timed_out"} {
		if !statuses[expected] {
			t.Errorf("run_status reads as %v, which no longer looks like the run lifecycle", statuses)
			break
		}
	}

	vocabularies, err := migrationVocabularies(root)
	if err != nil {
		t.Fatal(err)
	}
	if !vocabularies["cost_events.kind"]["match_reasons"] {
		t.Errorf("cost_events.kind reads as %v; the list a later migration widened was not the one kept", vocabularies["cost_events.kind"])
	}
	if _, kept := vocabularies["analytics_events.arrival"]; kept {
		t.Errorf("analytics_events.arrival was dropped by a migration but still reads as a vocabulary")
	}

	states, err := goConstStrings(
		filepath.Join(root, filepath.FromSlash("apps/platform/internal/creator/creation/state.go")), "State")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) < 10 {
		t.Errorf("creation declares %d states; the session vocabulary is larger than that", len(states))
	}
}

func TestDomainVocabularyNamesTheSourceMissingAValue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, "db/migrations/0001_runs.sql", `CREATE TYPE run_status AS ENUM (
    'queued',
    'running',
    'archived'
);
`)
	writeAt(t, root, "contract/gen.go", `package gen

type RunStatus string

const (
	RunStatusQueued  RunStatus = "queued"
	RunStatusRunning RunStatus = "running"
)
`)
	only := []domainVocabulary{{
		name: "run status",
		sources: []vocabularySource{
			postgresEnum("db/migrations/0001_runs.sql", "run_status"),
			goConstEnum("contract/gen.go", "RunStatus"),
		},
	}}
	problems := reconcileVocabularies(root, only)
	if len(problems) != 1 {
		t.Fatalf("expected exactly the one value the contract omits, got %d:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if !strings.Contains(problems[0], `"archived"`) || !strings.Contains(problems[0], "contract/gen.go") {
		t.Fatalf("the report must name the value and the source that lacks it: %s", problems[0])
	}
}

func TestDomainVocabularyReadsCodeNotComments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, "db/migrations/0001_runs.sql", `CREATE TYPE run_status AS ENUM (
    'queued'
);
`)
	writeAt(t, root, "contract/gen.go", `package gen

type RunStatus string

const (
	RunStatusQueued RunStatus = "queued"
)

// RunStatusArchived RunStatus = "archived"
`)
	only := []domainVocabulary{{
		name: "run status",
		sources: []vocabularySource{
			postgresEnum("db/migrations/0001_runs.sql", "run_status"),
			goConstEnum("contract/gen.go", "RunStatus"),
		},
	}}
	if problems := reconcileVocabularies(root, only); len(problems) != 0 {
		t.Fatalf("a value mentioned in a comment is not a declaration:\n%s", strings.Join(problems, "\n"))
	}
}

func TestDomainVocabularyRefusesASourceThatYieldsNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, "db/migrations/0001_runs.sql", `CREATE TYPE run_status AS ENUM (
    'queued'
);
`)
	writeAt(t, root, "contract/gen.go", `package gen

type RunStatus string
`)
	only := []domainVocabulary{{
		name: "run status",
		sources: []vocabularySource{
			postgresEnum("db/migrations/0001_runs.sql", "run_status"),
			goConstEnum("contract/gen.go", "RunStatus"),
		},
	}}
	problems := reconcileVocabularies(root, only)
	if len(problems) != 1 || !strings.Contains(problems[0], "declares no value") {
		t.Fatalf("a source that reads as empty must be reported, not silently agreed with:\n%s", strings.Join(problems, "\n"))
	}
}

func TestMigrationVocabularies(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		migrations map[string]string
		key        string
		want       []string
	}{
		{
			name: "the same column on two tables stays apart",
			migrations: map[string]string{"0001_ledger.sql": `CREATE TABLE entries (
    kind text NOT NULL CHECK (kind IN ('debit', 'grant'))
);
CREATE TABLE events (
    kind text NOT NULL CHECK (kind IN ('step', 'review'))
);
`},
			key:  "entries.kind",
			want: []string{"debit", "grant"},
		},
		{
			name: "a later migration replaces the list",
			migrations: map[string]string{
				"0001_events.sql": `CREATE TABLE events (
    kind text NOT NULL CHECK (kind IN ('step', 'review'))
);
`,
				"0002_more_kinds.sql": `ALTER TABLE events DROP CONSTRAINT events_kind_check;
ALTER TABLE events ADD CONSTRAINT events_kind_check CHECK (kind IN (
    'step', 'review', 'match'));
`,
			},
			key:  "events.kind",
			want: []string{"match", "review", "step"},
		},
		{
			name: "a nullable list is a vocabulary",
			migrations: map[string]string{"0001_events.sql": `CREATE TABLE events (
    ref_type text CHECK (ref_type IS NULL OR ref_type IN ('run', 'session'))
);
`},
			key:  "events.ref_type",
			want: []string{"run", "session"},
		},
		{
			name: "a null test on another column is not a vocabulary",
			migrations: map[string]string{"0001_events.sql": `CREATE TABLE events (
    CHECK (ref_id IS NULL OR ref_type IN ('run'))
);
`},
			key: "events.ref_type",
		},
		{
			name: "NOT IN is not a vocabulary",
			migrations: map[string]string{"0001_entries.sql": `CREATE TABLE entries (
    CONSTRAINT positive CHECK (
        kind NOT IN ('grant') OR delta > 0)
);
`},
			key: "entries.kind",
		},
		{
			name: "a dropped column leaves no vocabulary",
			migrations: map[string]string{
				"0001_events.sql": `CREATE TABLE events (
    arrival text CHECK (arrival IN ('search', 'direct'))
);
`,
				"0002_drop.sql": `ALTER TABLE events DROP COLUMN arrival;
`,
			},
			key: "events.arrival",
		},
		{
			name: "a renamed column carries its vocabulary",
			migrations: map[string]string{
				"0001_events.sql": `CREATE TABLE events (
    origin text CHECK (origin IN ('search', 'direct'))
);
`,
				"0002_rename.sql": `ALTER TABLE events RENAME COLUMN origin TO arrival;
`,
			},
			key:  "events.arrival",
			want: []string{"direct", "search"},
		},
		{
			name: "a comment inside the list is not a value",
			migrations: map[string]string{"0001_runs.sql": `ALTER TABLE runs ADD COLUMN failure_class text
    CHECK (failure_class IS NULL OR failure_class IN (
        'provider_error',   -- the provider's own fault (not ours)
        'timeout'           -- soft limit
    ));
`},
			key:  "runs.failure_class",
			want: []string{"provider_error", "timeout"},
		},
		{
			name: "two dashes inside a string do not start a comment",
			migrations: map[string]string{"0001_events.sql": `CREATE TABLE events (kind text);
COMMENT ON COLUMN events.kind IS 'a -- b'; ALTER TABLE events ADD CONSTRAINT k CHECK (kind IN ('step'));
`},
			key:  "events.kind",
			want: []string{"step"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, body := range c.migrations {
				writeAt(t, root, "db/migrations/"+name, body)
			}
			vocabularies, err := migrationVocabularies(root)
			if err != nil {
				t.Fatal(err)
			}
			values, declared := vocabularies[c.key]
			if c.want == nil {
				if declared {
					t.Fatalf("%s reads as a vocabulary %v; it should not", c.key, values)
				}
				return
			}
			var got []string
			for value := range values {
				got = append(got, value)
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("%s = %v, want %v", c.key, got, c.want)
			}
		})
	}
}

func TestSQLColumnCheckNamesTheColumnItCannotFind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, "db/migrations/0001_events.sql", `CREATE TABLE events (kind text CHECK (kind IN ('step')));
`)
	_, err := sqlColumnCheck("events", "origin").read(root)
	if err == nil || !strings.Contains(err.Error(), "events.origin") {
		t.Fatalf("a missing column must be reported by name, got %v", err)
	}
}

func readerFixture(t *testing.T, readerConstants string) []string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "db/migrations/0001_evaluations.sql", `CREATE TABLE evaluations (
    overall text CHECK (overall IN ('met', 'not_met'))
);
`)
	writeAt(t, root, "domain/overall.go", `package domain

type Overall string

const (
	OverallMet    Overall = "met"
	OverallNotMet Overall = "not_met"
)
`)
	writeAt(t, root, "consumer/overall.go", "package consumer\n\ntype overall string\n\n"+readerConstants)
	return reconcileVocabularies(root, []domainVocabulary{{
		name:    "evaluation overall",
		sources: []vocabularySource{sqlColumnCheck("evaluations", "overall"), goConstEnum("domain/overall.go", "Overall")},
		readers: []vocabularySource{goConstEnum("consumer/overall.go", "overall")},
	}})
}

func TestDomainVocabularyLetsAReaderNameOnlyTheValuesItReads(t *testing.T) {
	t.Parallel()
	if problems := readerFixture(t, "const overallMet overall = \"met\"\n"); len(problems) != 0 {
		t.Fatalf("a reader naming one value of the vocabulary was refused:\n%s", strings.Join(problems, "\n"))
	}
}

func TestDomainVocabularyRefusesAReaderNamingAValueNoSourceDeclares(t *testing.T) {
	t.Parallel()
	problems := readerFixture(t, "const (\n\toverallMet overall = \"met\"\n\toverallPassed overall = \"passed\"\n)\n")
	if len(problems) != 1 || !strings.Contains(problems[0], `"passed"`) || !strings.Contains(problems[0], "consumer/overall.go") {
		t.Fatalf("the report must name the value and the reader that spells it:\n%s", strings.Join(problems, "\n"))
	}
}

func TestDomainVocabularyRefusesAReaderThatNamesNothing(t *testing.T) {
	t.Parallel()
	problems := readerFixture(t, "")
	if len(problems) != 1 || !strings.Contains(problems[0], "declares no value") {
		t.Fatalf("a reader that reads as empty must be reported, not silently agreed with:\n%s", strings.Join(problems, "\n"))
	}
}

func TestDomainVocabularyReadsAListDeclaredAsAFunction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, "domain/state.go", `package domain

type State string

const (
	StateQueued State = "queued"
	StateDone   State = "done"
)

func AllStates() []State {
	return []State{
		StateQueued,
	}
}
`)
	only := []domainVocabulary{{
		name: "session state",
		sources: []vocabularySource{
			goConstEnum("domain/state.go", "State"),
			goListedConstEnum("domain/state.go", "AllStates", "domain/state.go", "State"),
		},
	}}
	problems := reconcileVocabularies(root, only)
	if len(problems) != 1 || !strings.Contains(problems[0], `"done"`) {
		t.Fatalf("a constant the list forgets must be reported:\n%s", strings.Join(problems, "\n"))
	}
}
