package skillpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTheRepoOwnSkillsPassItsOwnValidator(t *testing.T) {
	t.Parallel()
	skills := filepath.Join(repoRoot(t), ".claude", "skills")
	entries, err := os.ReadDir(skills)
	if err != nil {
		t.Fatalf("no .claude/skills at the repo root: %v", err)
	}
	seen := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		seen++
		errs := Validate(os.DirFS(filepath.Join(skills, e.Name()))).Categorize().Errors
		for _, f := range errs {
			t.Errorf(".claude/skills/%s: %s %s: %s", e.Name(), f.Code, f.Path, f.Message)
		}
	}
	if seen == 0 {
		t.Fatal("no skill directories found; the test has lost its subject")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, errA := os.Stat(filepath.Join(dir, "AGENTS.md"))
		_, errC := os.Stat(filepath.Join(dir, ".claude"))
		if errA == nil && errC == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (AGENTS.md + .claude/) not found above " + dir)
		}
		dir = parent
	}
}
