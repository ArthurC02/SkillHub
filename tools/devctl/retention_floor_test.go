package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRetention(t *testing.T, retention, env string) string {
	t.Helper()
	return writeRetentionFull(t, retention, manifestSQL, env, defaultWindowDoc)
}

const defaultWindowDoc = "## 8. 附錄\n\n### 8.2 B 版：封閉測試（14 天，自己使用）\n\n內文。\n"

func writeRetentionFull(t *testing.T, retention, sql, env, window string) string {
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
	write(runArtifactRetentionPackage+"/job.go", retention)
	write(runArtifactRetentionPackage+"/job_test.go", "package run\n\nimport \"time\"\n\nconst runArtifactRetention = time.Hour\n")
	write("db/queries/runs.sql", sql)
	write(envExampleDoc, env)
	write(observationWindowDoc, window)
	return root
}

func retentionGo(expr string) string {
	return "package run\n\nimport \"time\"\n\n" +
		"// The retention used to be declared as 999 * 24 * time.Hour and this sentence has outlived it.\n" +
		"const runArtifactRetention = " + expr + "\n"
}

const ninetyDays = "90 * 24 * time.Hour"

const manifestSQL = "-- name: InsertRunArtifact :exec\n" +
	"INSERT INTO artifacts (workspace_id, run_id, kind, file_name, object_key, expires_at)\n" +
	"VALUES (@workspace_id, @run_id, 'run_output', @file_name, @object_key, @expires_at);\n"

const allFloorsMet = "METRICS_ADDR=\n" +
	"# Trace event retention.\n" +
	"TRACE_RETENTION=2160h\n" +
	"DOWNLOAD_ARTIFACT_RETENTION=720h\n" +
	"ANALYTICS_RETENTION=8760h\n" +
	"RUN_QUOTA=off\n"

func TestRetentionFloorAcceptsATreeWhereAllThreeFloorsAreMet(t *testing.T) {
	t.Parallel()
	root := writeRetention(t, retentionGo(ninetyDays), allFloorsMet)
	if problems := retentionFloorProblems(root); len(problems) != 0 {
		t.Fatalf("a tree meeting all three floors was rejected: %v", problems)
	}
}

func TestRetentionFloorReadsTheConstantHoweverItsArithmeticIsWritten(t *testing.T) {
	t.Parallel()
	for _, expr := range []string{"90 * 24 * time.Hour", "(90 * 24) * time.Hour", "2160 * time.Hour", "129600 * time.Minute"} {
		root := writeRetention(t, retentionGo(expr), allFloorsMet)
		if problems := retentionFloorProblems(root); len(problems) != 0 {
			t.Fatalf("%s: %v", expr, problems)
		}
		if _, retention, _ := runArtifactRetention(root); retention != 90*24*time.Hour {
			t.Fatalf("%s read as %v, want 90 days", expr, retention)
		}
	}
}

func TestRetentionFloorSpeaksForEachOfTheThreeRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, retention, env, window, want string
	}{{

		name:      "rule 2: run artifact below the re-evaluation window",
		retention: "30 * 24 * time.Hour",
		env:       allFloorsMet,
		want:      "rule 2 requires Run Artifact retention",
	}, {

		name:      "rule 2: one hour under the re-evaluation window",
		retention: "2159 * time.Hour",
		env:       allFloorsMet,
		want:      "rule 2 requires Run Artifact retention",
	}, {

		name:      "rule 1: download retention below the observation window",
		retention: ninetyDays,
		env:       strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=168h", 1),
		want:      "rule 1 requires download retention",
	}, {

		name:      "rule 1: the study got longer and nothing else moved",
		retention: ninetyDays,
		env:       allFloorsMet,
		window:    "### 8.2 B 版：封閉測試（60 天，自己使用）\n",
		want:      "rule 1 requires download retention",
	}, {
		name:      "rule 3: analytics below one complete funnel",
		retention: ninetyDays,
		env:       strings.Replace(allFloorsMet, "ANALYTICS_RETENTION=8760h", "ANALYTICS_RETENTION=720h", 1),
		want:      "rule 3 requires analytics retention",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			window := tc.window
			if window == "" {
				window = defaultWindowDoc
			}
			root := writeRetentionFull(t, retentionGo(tc.retention), manifestSQL, tc.env, window)
			problems := retentionFloorProblems(root)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestRetentionFloorIsAFloorAndNotAThreshold(t *testing.T) {
	t.Parallel()
	at := strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=336h", 1)
	root := writeRetention(t, retentionGo(ninetyDays), at)
	if problems := retentionFloorProblems(root); len(problems) != 0 {
		t.Fatalf("exactly 14 days against a 14-day window was rejected: %v", problems)
	}
	under := strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h", "DOWNLOAD_ARTIFACT_RETENTION=335h", 1)
	root = writeRetention(t, retentionGo(ninetyDays), under)
	if problems := retentionFloorProblems(root); len(problems) != 1 {
		t.Fatalf("one hour under the window was accepted: %v", problems)
	}
}

func TestRetentionFloorSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	ninety := retentionGo(ninetyDays)
	for _, tc := range []struct {
		name, retention, sql, env, window, want string
	}{{
		name:      "no retention constant at all",
		retention: "package run\n",
		env:       allFloorsMet, want: "has lost its subject",
	}, {
		name:      "the SQL stamps a retention of its own again",
		retention: ninety,
		sql: "-- name: InsertRunArtifact :exec\n" +
			"INSERT INTO artifacts (workspace_id, run_id, kind, expires_at)\n" +
			"VALUES (@workspace_id, @run_id, 'run_output', now() + interval '90 days');\n",
		env: allFloorsMet, want: "there are now two authors of it",
	}, {
		name:      "the SQL adds a deployment parameter to now()",
		retention: ninety,
		sql: "-- name: InsertRunArtifact :exec\n" +
			"INSERT INTO artifacts (workspace_id, run_id, kind, expires_at)\n" +
			"VALUES (@workspace_id, @run_id, 'run_output', now() + @retention);\n",
		env: allFloorsMet, want: "there are now two authors of it",
	}, {
		name:      "two declarations of the constant",
		retention: ninety + "\nconst runArtifactRetention = 30 * 24 * time.Hour\n",
		env:       allFloorsMet, want: "there are now two authors of it",
	}, {
		name:      "the constant is no longer arithmetic on literals",
		retention: "package run\n\nconst runArtifactRetention = retentionFromEnvironment\n",
		env:       allFloorsMet, want: "something other than a positive product",
	}, {
		name:      "TRACE_RETENTION is gone",
		retention: ninety,
		env:       strings.Replace(allFloorsMet, "TRACE_RETENTION=2160h\n", "", 1),
		want:      "no longer assigns TRACE_RETENTION",
	}, {
		name:      "DOWNLOAD_ARTIFACT_RETENTION is gone",
		retention: ninety,
		env:       strings.Replace(allFloorsMet, "DOWNLOAD_ARTIFACT_RETENTION=720h\n", "", 1),
		want:      "no longer assigns DOWNLOAD_ARTIFACT_RETENTION",
	}, {
		name:      "ANALYTICS_RETENTION is gone",
		retention: ninety,
		env:       strings.Replace(allFloorsMet, "ANALYTICS_RETENTION=8760h\n", "", 1),
		want:      "no longer assigns ANALYTICS_RETENTION",
	}, {
		name:      "two assignments of one window",
		retention: ninety,
		env:       allFloorsMet + "TRACE_RETENTION=1h\n",
		want:      "cannot have two values",
	}, {
		name:      "the observation window heading is gone",
		retention: ninety,
		env:       allFloorsMet,
		window:    "### 8.2 B 版\n\n長度搬到別處了。\n",
		want:      "lost half its subject",
	}, {
		name:      "two closed-beta lengths",
		retention: ninety,
		env:       allFloorsMet,
		window:    defaultWindowDoc + "### 8.3 C 版：封閉測試（21 天）\n",
		want:      "cannot have two lengths",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			window := tc.window
			if window == "" {
				window = defaultWindowDoc
			}
			sql := tc.sql
			if sql == "" {
				sql = manifestSQL
			}
			problems := retentionFloorProblems(writeRetentionFull(t, tc.retention, sql, tc.env, window))
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

func TestRetentionFloorIgnoresCommentsAndTests(t *testing.T) {
	t.Parallel()
	source := "package run\n\nimport \"time\"\n\n" +
		"// const runArtifactRetention = 30 * 24 * time.Hour\n" +
		"const runArtifactRetention = 90 * 24 * time.Hour\n"
	root := writeRetention(t, source, allFloorsMet)
	if problems := retentionFloorProblems(root); len(problems) != 0 {
		t.Fatalf("a comment or a test file voted on the value: %v", problems)
	}
}

func TestTheRealRetentionFloorsAreMet(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := retentionFloorProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}

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
