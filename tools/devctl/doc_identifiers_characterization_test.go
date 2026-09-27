package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDocIdentifierFilesListsOnlyMarkdownUnderItsTrees(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, relative := range []string{"docs/adr/decision.md", "docs/adr/notes.txt"} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, problems := docIdentifierFiles(root)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if !slices.Contains(files, "docs/adr/decision.md") || slices.Contains(files, "docs/adr/notes.txt") {
		t.Fatalf("files = %v, want the markdown file and not the text file", files)
	}
}
