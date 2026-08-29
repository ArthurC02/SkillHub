package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tree in the shape the real one has: one sqlc query file, one .env template,
// and the document that states the observation window.
func writeRetention(t *testing.T, sql, env string) string {
	t.Helper()
	return writeRetentionFull(t, sql, env, defaultWindowDoc)
}

const defaultWindowDoc = "## 8. 附錄\n\n### 8.2 B 版：封閉測試（14 天，自己使用）\n\n內文。\n"

func writeRetentionFull(t *testing.T, sql, env, window string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("db/queries/runs.sql", sql)
	write(envExampleDoc, env)
	write(observationWindowDoc, window)
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

// Every value at or above its floor, which is where the tree stands now that
// R-11 raised the SQL literal to 90 days.
const allFloorsMet = "METRICS_ADDR=\n" +
	"# Trace event retention.\n" +
	"TRACE_RETENTION=2160h\n" +
	"DOWNLOAD_ARTIFACT_RETENTION=720h\n" +
	"ANALYTICS_RETENTION=8760h\n" +
	"RUN_QUOTA=off\n"

func TestRetentionFloorAcceptsATreeWhereAllThreeFloorsAreMet(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), allFloorsMet)
	if problems := retentionFloorProblems(root); len(problems) != 0 {
		t.Fatalf("a tree meeting all three floors was rejected: %v", problems)
	}
}

// Rule 2 is anchored to what the statement does, never to its name: the name has
// already changed once under this check's feet.
func TestRetentionFloorDoesNotDependOnTheQueryName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"InsertRunArtifact", "RecordRunArtifact", "SomethingElseEntirely"} {
		root := writeRetention(t, runArtifactSQL(name, "now() + interval '90 days'"), allFloorsMet)
		if problems := retentionFloorProblems(root); len(problems) != 0 {
			t.Fatalf("%s: %v", name, problems)
		}
	}
}

// One subtest per floor, each breaking only its own number. Before 2026-08-29
// only the first of these could go red.
func TestRetentionFloorSpeaksForEachOfTheThreeRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sqlExpiry, env, window, want string
	}{{
		// The shortfall R-11 closed. It must fail again if anyone reopens it.
		name:      "rule 2: run artifact below the re-evaluation window",
		sqlExpiry: "now() + interval '30 days'",
		env:       allFloorsMet,
		want:      "rule 2 requires Run Artifact retention",
	}, {
		// The value that was actually set, and corrected the same day.
		name:      "rule 1: download retention below the observation window",
		sqlExpiry: "now() + interval '90 days'",
		env:       strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=168h", 1),
		want:      "rule 1 requires download retention",
	}, {
		// A longer study with the same retention breaks the same rule from the
		// other side, which is why the 14 is parsed and not copied.
		name:      "rule 1: the study got longer and nothing else moved",
		sqlExpiry: "now() + interval '90 days'",
		env:       allFloorsMet,
		window:    "### 8.2 B 版：封閉測試（60 天，自己使用）\n",
		want:      "rule 1 requires download retention",
	}, {
		name:      "rule 3: analytics below one complete funnel",
		sqlExpiry: "now() + interval '90 days'",
		env:       strings.Replace(allFloorsMet, "ANALYTICS_RETENTION=8760h", "ANALYTICS_RETENTION=720h", 1),
		want:      "rule 3 requires analytics retention",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			window := tc.window
			if window == "" {
				window = defaultWindowDoc
			}
			root := writeRetentionFull(t, runArtifactSQL("InsertRunArtifact", tc.sqlExpiry), tc.env, window)
			problems := retentionFloorProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// Exactly at the floor is met, one hour under is not. Both, because 「三條都是
// 下界」 and a `<=` here would let a package expire on the last morning of the
// study.
func TestRetentionFloorIsAFloorAndNotAThreshold(t *testing.T) {
	t.Parallel()
	at := strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=336h", 1)
	root := writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), at)
	if problems := retentionFloorProblems(root); len(problems) != 0 {
		t.Fatalf("exactly 14 days against a 14-day window was rejected: %v", problems)
	}
	under := strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=335h", 1)
	root = writeRetention(t, runArtifactSQL("InsertRunArtifact", "now() + interval '90 days'"), under)
	if problems := retentionFloorProblems(root); len(problems) != 1 {
		t.Fatalf("one hour under the window was accepted: %v", problems)
	}
}

// Every way this check passes while comparing nothing.
func TestRetentionFloorSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	ninety := "now() + interval '90 days'"
	for _, tc := range []struct {
		name, sql, env, window, want string
	}{{
		name: "no run_output INSERT at all",
		sql:  "-- name: ListRunArtifacts :many\nSELECT * FROM artifacts;\n",
		env:  allFloorsMet, want: "has lost its subject",
	}, {
		name: "two statements stamp the retention",
		sql:  runArtifactSQL("InsertRunArtifact", ninety) + runArtifactSQL("AlsoInsert", ninety),
		env:  allFloorsMet, want: "there are now two authors of it",
	}, {
		name: "the literal became a deployment parameter",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + @retention"),
		env:  allFloorsMet, want: "rather than a literal",
	}, {
		name: "an interval unit with no fixed length",
		sql:  runArtifactSQL("InsertRunArtifact", "now() + interval '3 months'"),
		env:  allFloorsMet, want: "has no fixed length",
	}, {
		name: "TRACE_RETENTION is gone",
		sql:  runArtifactSQL("InsertRunArtifact", ninety),
		env:  strings.Replace(allFloorsMet, "TRACE_RETENTION=2160h\n", "", 1),
		want: "no longer assigns TRACE_RETENTION",
	}, {
		name: "DOWNLOAD_ARTIFACT_RETENTION is gone",
		sql:  runArtifactSQL("InsertRunArtifact", ninety),
		env:  strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h\n", "", 1),
		want: "no longer assigns DOWNLOAD_ARTIFACT_RETENTION",
	}, {
		name: "ANALYTICS_RETENTION is gone",
		sql:  runArtifactSQL("InsertRunArtifact", ninety),
		env:  strings.Replace(allFloorsMet, "ANALYTICS_RETENTION=8760h\n", "", 1),
		want: "no longer assigns ANALYTICS_RETENTION",
	}, {
		name: "two assignments of one window",
		sql:  runArtifactSQL("InsertRunArtifact", ninety),
		env:  allFloorsMet + "TRACE_RETENTION=1h\n",
		want: "cannot have two values",
	}, {
		name:   "the observation window heading is gone",
		sql:    runArtifactSQL("InsertRunArtifact", ninety),
		env:    allFloorsMet,
		window: "### 8.2 B 版\n\n長度搬到別處了。\n",
		want:   "lost half its subject",
	}, {
		name:   "two closed-beta lengths",
		sql:    runArtifactSQL("InsertRunArtifact", ninety),
		env:    allFloorsMet,
		window: defaultWindowDoc + "### 8.3 C 版：封閉測試（21 天）\n",
		want:   "cannot have two lengths",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			window := tc.window
			if window == "" {
				window = defaultWindowDoc
			}
			problems := retentionFloorProblems(writeRetentionFull(t, tc.sql, tc.env, window))
			if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
	t.Run("no tree at all", func(t *testing.T) {
		t.Parallel()
		if problems := retentionFloorProblems(t.TempDir()); len(problems) == 0 {
			t.Fatal("an empty tree was accepted")
		}
	})
}

// runs.sql discusses run_output and intervals in three comment blocks. A version
// that read comments would take one of them as the value.
func TestRetentionFloorIgnoresComments(t *testing.T) {
	t.Parallel()
	sql := "-- name: InsertRunArtifact :execrows\n" +
		"-- This used to stamp now() + interval '30 days' and no longer does.\n" +
		"INSERT INTO artifacts (workspace_id, run_id, kind, file_name, object_key, expires_at)\n" +
		"SELECT @workspace_id, @run_id, 'run_output', @file_name, @object_key, now() + interval '90 days'\n" +
		"WHERE NOT EXISTS (SELECT 1 FROM artifacts WHERE run_id = @run_id);\n"
	if problems := retentionFloorProblems(writeRetention(t, sql, allFloorsMet)); len(problems) != 0 {
		t.Fatalf("a comment voted on the value: %v", problems)
	}
}

// Pointed at the tree: all three floors, against the real numbers.
func TestTheRealRetentionFloorsAreMet(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := retentionFloorProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
	// Guard the reach of each half separately: the check above is satisfied by
	// finding nothing on both sides of a comparison.
	if _, artifact, where := runArtifactRetention(root); artifact <= 0 {
		t.Errorf("run artifact retention read as %v from %q", artifact, where)
	}
	for _, name := range []string{"TRACE_RETENTION", "DOWNLOAD_ARTIFACT_RETENTION", "ANALYTICS_RETENTION"} {
		if _, d := envRetention(root, name); d <= 0 {
			t.Errorf("%s read as %v", name, d)
		}
	}
	if _, window := observationWindow(root); window < 24*time.Hour {
		t.Errorf("observation window read as %v", window)
	}
}
