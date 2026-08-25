package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every checker on the roster must actually reach the report.
//
// Until this existed, automationCheck had no test at all: each checker was
// tested by calling it directly, and the seven `append` lines that ran them were
// covered by nothing. Deleting the queryOwnerProblems line kept `go test ./...`
// green while ADR-033's ratchet quietly stopped running, and the same was true
// of every other line.
//
// The fixture is a repo broken in every direction on purpose, so each checker has
// something to say. Two assertions, and the second is the one with teeth: the
// checker must produce a problem here (otherwise the wiring assertion would be
// vacuously true), and every problem it produces must appear in the output.
// The roster itself, named. TestAutomationCheckRunsEveryChecker below walks
// documentCheckers(), so deleting an entry would delete its subtest with it and
// stay green — the same shape as the hole it was written to close. This is the
// second author the list needs: removing a checker is now two edits, and one of
// them is in a test file where "why is this line going away" has to be answered.
func TestDocumentCheckerRosterIsComplete(t *testing.T) {
	t.Parallel()
	want := []string{
		"drift-marker", "depguard-deny", "one-number", "query-owner",
		"context-map", "doc-identifier", "milestone-tally", "backlog-tally",
		"baseline-tally", "retention-floor", "sdk-version",
	}
	got := make([]string, 0, len(want))
	for _, checker := range documentCheckers() {
		got = append(got, checker.name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the checker roster changed: got [%s], want [%s]. "+
			"Adding a checker means adding it here; removing one means saying so here too",
			strings.Join(got, ", "), strings.Join(want, ", "))
	}
}

func TestAutomationCheckRunsEveryChecker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// automationCheck returns early when the Taskfile cannot be read, before any
	// checker runs; everything else is deliberately missing or wrong.
	write("Taskfile.yml", "version: \"3\"\ntasks:\n")
	// doc-identifier only speaks when a live document names something no file
	// declares, so give it one.
	write("AGENTS.md", "AGENTS 導覽：`NoSuchSymbolAnywhere` 早就被刪掉了。\n")
	// drift-marker returns on the first unreadable file and reads its two sources
	// out of a map, so with both missing its message depends on map order. Both
	// present and disagreeing gives it one thing to say.
	write("apps/platform/.golangci.yml", "# drift: DDD-005 (run -> eval)\n")
	write("docs/adr/"+contextMapADR, "# ADR-032\n\n沒有 §1 表格，也沒有附錄 A。\n")

	var out bytes.Buffer
	if err := automationCheck(root, &out); err == nil {
		t.Fatal("a repo missing every input was accepted")
	}
	report := out.String()

	for _, checker := range documentCheckers() {
		t.Run(checker.name, func(t *testing.T) {
			problems := checker.check(root)
			if len(problems) == 0 {
				t.Fatalf("%s reported nothing on a broken tree, so the wiring assertion below proves nothing",
					checker.name)
			}
			for _, problem := range problems {
				if !strings.Contains(report, problem) {
					t.Fatalf("%s is not wired into automationCheck: %q is missing from the report",
						checker.name, problem)
				}
			}
		})
	}
}

func TestDriftMarkerProblems(t *testing.T) {
	t.Parallel()
	// The ADR fixture carries the `DDD-00x` placeholder from ADR-032 §3 on
	// purpose: it is prose, not a marker, and must not be counted.
	write := func(root, lint, adr string) {
		lintPath := filepath.Join(root, "apps", "platform", ".golangci.yml")
		adrPath := filepath.Join(root, "docs", "adr", "ADR-032-ddd-bounded-context-governance-for-platform.md")
		for path, contents := range map[string]string{lintPath: lint, adrPath: adr} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	matched := t.TempDir()
	write(matched,
		"# drift: DDD-005 (run -> eval)\n# drift: DDD-006 (run -> ingest)\n# drift: DDD-006 (eval -> ingest)\n",
		"標註（`# drift: DDD-00x`）\n| **drift: DDD-005** |\n| **drift: DDD-006** |\n| **drift: DDD-006** |\n")
	if problems := driftMarkerProblems(matched); len(problems) != 0 {
		t.Fatalf("matching markers reported problems: %#v", problems)
	}

	skewed := t.TempDir()
	write(skewed,
		"# drift: DDD-005 (run -> eval)\n# drift: DDD-006 (run -> ingest)\n# drift: DDD-006 (eval -> ingest)\n",
		"| **drift: DDD-005** |\n| **drift: DDD-006** |\n")
	problems := driftMarkerProblems(skewed)
	if len(problems) != 1 {
		t.Fatalf("expected one problem for a marker only present in the lint config, got %#v", problems)
	}
	if !strings.Contains(problems[0], "lint=2 adr=1") {
		t.Fatalf("problem does not report the count difference: %q", problems[0])
	}
}

func TestTaskDescriptionsFindsMissingDescriptions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "Taskfile.yml")
	contents := `version: "3"
tasks:
  documented:
    desc: Safe task
    cmd: echo ok
  missing:
    cmd: echo hidden
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskDescriptions(path)
	if err != nil {
		t.Fatal(err)
	}
	if tasks["documented"] != "Safe task" {
		t.Fatalf("documented description = %q", tasks["documented"])
	}
	if description, ok := tasks["missing"]; !ok || description != "" {
		t.Fatalf("missing task was not detected: %#v", tasks)
	}
}
