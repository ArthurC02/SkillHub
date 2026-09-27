package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheCommentBudgetSkipsHiddenDirectoriesButReadsRootFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const fourLines = "// one\n// two\n// three\n// four\nfunc f() {}\n"
	for relative, contents := range map[string]string{
		"apps/.cache/hidden.go": "package x\n\n" + fourLines,
		"root.go":               "package x\n\n" + fourLines,
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	problems := strings.Join(commentBudgetProblems(root), "\n")
	if strings.Contains(problems, "hidden.go") {
		t.Fatalf("a file under a hidden directory was scanned: %s", problems)
	}
	if !strings.Contains(problems, "root.go has 1") {
		t.Fatalf("a file at the repository root was not scanned: %s", problems)
	}
}
