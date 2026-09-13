package main

import (
	"path/filepath"
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
