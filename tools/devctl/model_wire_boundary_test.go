package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheRealTreeKeepsTheModelWireBehindItsAdapters(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := modelWireBoundaryProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
}

func TestModelWireBoundaryNoticesADomainFileReachingForTheWire(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path string
		wantFound  bool
	}{
		{"a domain file", "apps/platform/internal/trial/design/leak.go", true},
		{"an adapter beside the port", "apps/platform/internal/trial/design/leak_adapter.go", false},
		{"a test", "apps/platform/internal/trial/design/leak_test.go", false},
		{"the composition root", "apps/platform/internal/entrypoint/worker/leak.go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := treeWithFile(t, tc.path, "package x\n\nimport _ \""+modelWirePackage+"\"\n")
			problems := modelWireBoundaryProblems(root)
			if tc.wantFound {
				if len(problems) != 1 || !strings.Contains(problems[0], tc.path) {
					t.Fatalf("a domain file importing the wire package produced %v", problems)
				}
				return
			}
			if len(problems) != 0 {
				t.Fatalf("%s was reported: %v", tc.name, problems)
			}
		})
	}
}

func TestModelWireBoundaryIgnoresAnUnrelatedImport(t *testing.T) {
	t.Parallel()
	root := treeWithFile(t, "apps/platform/internal/trial/design/other.go",
		"package x\n\nimport _ \"context\"\n")
	if problems := modelWireBoundaryProblems(root); len(problems) != 0 {
		t.Fatalf("an unrelated import was reported: %v", problems)
	}
}

func treeWithFile(t *testing.T, path, content string) string {
	t.Helper()
	root := t.TempDir()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
