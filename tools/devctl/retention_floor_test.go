package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tree in the shape the real one has: one sqlc query file and one .env
// template.
func writeRetention(t *testing.T, sql, env string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "db", "queries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, envExampleDoc), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// The real statement, trimmed: doc comments above it that talk about run_output
// and an interval, then the INSERT. The comments are here on purpose — they are
// the reason sqlStatements drops comment lines, and a version that did not would
// pass this file's other tests while reading the prose as the value.
func runArtifactSQL(name, expiry string) string {
	return "-- name: ListRunArtifacts :many\n" +
		"-- The manifest, whose rows carry the retention stamped below.\n" +
		"SELECT * FROM artifacts WHERE run_id = $1 AND kind = 'run_output';\n" +
		"\n" +
		"-- name: " + name + " :execrows\n" +
		"-- Recorded when the run settles. The retention used to be written\n" +
		"-- now() + interval '999 days' and this sentence has outlived it.\n" +
		"INSERT INTO artifacts (workspace_id, run_id, kind, file_name, object_key, expires_at)\n" +
		"SELECT @workspace_id, @run_id, 'run_output', @file_name, @object_key, " + expiry + "\n" +
		"WHERE NOT EXISTS (SELECT 1 FROM artifacts WHERE run_id = @run_id);\n"
}

const traceEnv = "METRICS_ADDR=\n# Trace event retention. 3 個月.\nTRACE_RETENTION=2160h\nRUN_QUOTA=off\n"

var ninetyDayGap = &retentionShortfall{
	artifact: 30 * 24 * time.Hour,
	trace:    2160 * time.Hour,
	tracked:  "03:EVAL-014",
}

func TestRetentionFloorAcceptsAFloorThatIsMet(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), traceEnv)
	if problems := retentionFloorProblemsFor(root, nil); len(problems) != 0 {
		t.Fatalf("90 days against a 90 day window was rejected: %v", problems)
	}
}

// The check must survive the rename it has already survived once: the query was
// RecordRunArtifact when this was commissioned and is InsertRunArtifact now. A
// version anchored to the name is green here and blind in production.
func TestRetentionFloorDoesNotDependOnTheQueryName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"RecordRunArtifact", "InsertRunArtifact", "StoreOutputManifest"} {
		root := writeRetention(t, runArtifactSQL(name, "now() + interval '90 days'"), traceEnv)
		if problems := retentionFloorProblemsFor(root, nil); len(problems) != 0 {
			t.Fatalf("%s was rejected: %v", name, problems)
		}
	}
}

func TestRetentionFloorRejectsAShortfallNobodyDeclared(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"), traceEnv)
	problems := retentionFloorProblemsFor(root, nil)
	joined := strings.Join(problems, "\n")
	for _, want := range []string{
		"requires Run Artifact retention >= the re-evaluation window",
		"stamps 720h0m0s",
		"TRACE_RETENTION=2160h0m0s",
		"1440h0m0s short",
		"no shortfall is declared",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("want a problem containing %q, got %v", want, problems)
		}
	}
}

// The declared shortfall pins BOTH numbers, so it cannot absorb a change to
// either. This is the half that keeps the exception from becoming a hole.
func TestRetentionFloorRejectsAShortfallThatMoved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sql, env string
	}{{
		name: "the artifact side got shorter",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '7 days'"),
		env:  traceEnv,
	}, {
		name: "the artifact side got longer but is still short",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '60 days'"),
		env:  traceEnv,
	}, {
		name: "the trace side got longer",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"),
		env:  "TRACE_RETENTION=4320h\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRetention(t, tc.sql, tc.env)
			problems := retentionFloorProblemsFor(root, ninetyDayGap)
			if len(problems) == 0 {
				t.Fatal("a shortfall the declaration does not pin was accepted")
			}
			if !strings.Contains(strings.Join(problems, "\n"), "the only shortfall declared") {
				t.Fatalf("the problem does not name the declaration it failed to match: %v", problems)
			}
		})
	}
}

func TestRetentionFloorAcceptsExactlyTheDeclaredShortfall(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"), traceEnv)
	if problems := retentionFloorProblemsFor(root, ninetyDayGap); len(problems) != 0 {
		t.Fatalf("the declared 30/90 gap was rejected: %v", problems)
	}
}

// Closing the gap and leaving the declaration behind is the same silent-subject
// failure from the other side: the exception would then be sitting there ready
// to absorb the next 30/90 regression without a word.
func TestRetentionFloorRejectsADeclarationOfAGapThatIsGone(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), traceEnv)
	problems := retentionFloorProblemsFor(root, ninetyDayGap)
	if len(problems) == 0 {
		t.Fatal("a stale shortfall declaration was accepted")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "set nfr002aKnownShortfall to nil") {
		t.Fatalf("the problem does not say what to delete: %v", problems)
	}
}

// The whole point. A check that stops finding its subject and passes anyway is
// what this repository keeps being bitten by, so each of these must speak.
func TestRetentionFloorSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sql, env, want string
	}{{
		name: "the run-output insert is gone",
		sql:  "-- name: ListRunArtifacts :many\nSELECT * FROM artifacts WHERE kind = 'run_output';\n",
		env:  traceEnv,
		want: "This check has lost its subject",
	}, {
		name: "two statements stamp a retention",
		sql: runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'") +
			runArtifactSQL("InsertRunArtifactV2", "now() + interval '90 days'"),
		env:  traceEnv,
		want: "there are now two authors of it",
	}, {
		name: "the literal became a deployment parameter",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + @artifact_retention"),
		env:  traceEnv,
		want: "the value now lives in a deployment",
	}, {
		name: "the literal became a positional parameter",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + $7"),
		env:  traceEnv,
		want: "the value now lives in a deployment",
	}, {
		name: "expires_at is written some third way",
		sql:  runArtifactSQL("InsertRunArtifact", "make_expiry(@run_id)"),
		env:  traceEnv,
		want: "has been reworded",
	}, {
		name: "the retention is in a unit with no fixed length",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '3 months'"),
		env:  traceEnv,
		want: "has no fixed length",
	}, {
		name: "TRACE_RETENTION is gone from the template",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"),
		env:  "METRICS_ADDR=\nRUN_QUOTA=off\n",
		want: "lost half its subject",
	}, {
		name: "TRACE_RETENTION has two values",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"),
		env:  traceEnv + "TRACE_RETENTION=720h\n",
		want: "cannot have two values",
	}, {
		name: "TRACE_RETENTION is not a duration",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '30 days'"),
		env:  "TRACE_RETENTION=90d\n",
		want: "is not a Go duration",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRetention(t, tc.sql, tc.env)
			problems := retentionFloorProblemsFor(root, ninetyDayGap)
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}

	t.Run("no db/queries at all", func(t *testing.T) {
		if problems := retentionFloorProblemsFor(t.TempDir(), ninetyDayGap); len(problems) == 0 {
			t.Fatal("an empty tree was accepted")
		}
	})
}

// A commented-out value must not vote. The fixture's ListRunArtifacts comment
// says 999 days; if comments counted, that statement would match the anchor and
// the check would compare a sentence.
func TestRetentionFloorIgnoresComments(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), traceEnv)
	problems, retention, where := runArtifactRetention(root)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if retention != 90*24*time.Hour {
		t.Fatalf("read %s, want 2160h — a comment voted", retention)
	}
	if !strings.HasSuffix(where, ":9") {
		t.Fatalf("reported site %q, want the INSERT's line", where)
	}
}

// The real tree, on purpose. The unit tests above run on fixtures, and a fixture
// cannot notice that production's shape drifted out from under the anchor — the
// exact failure that turned RecordRunArtifact into InsertRunArtifact with nothing
// to say about it. This asserts only that both numbers are still FINDABLE, not
// what they are: what they are is retentionFloorProblems' business, and pinning
// it twice would mean two edits every time the values legitimately change.
func TestRetentionFloorFindsBothNumbersInTheRealTree(t *testing.T) {
	t.Parallel()
	// ../.. is the repo root, the same anchor shared_number_test.go uses.
	const root = "../.."
	problems, artifact, where := runArtifactRetention(root)
	if len(problems) != 0 {
		t.Fatalf("the run-output retention is no longer readable from db/queries: %v", problems)
	}
	if artifact <= 0 {
		t.Fatalf("run-output retention read as %s from %s", artifact, where)
	}
	problems, trace := traceRetention(root)
	if len(problems) != 0 {
		t.Fatalf("TRACE_RETENTION is no longer readable from %s: %v", envExampleDoc, problems)
	}
	if trace <= 0 {
		t.Fatalf("TRACE_RETENTION read as %s", trace)
	}
}
