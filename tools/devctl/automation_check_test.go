package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
