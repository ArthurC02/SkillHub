package skillpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTheRepoOwnSkillsPassItsOwnValidator(t *testing.T) {
	t.Parallel()
	manifests, err := filepath.Glob(filepath.Join(repoRoot(t), ".claude", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatal("no .claude/skills/*/SKILL.md found; the test has lost its subject")
	}
	for _, manifest := range manifests {
		dir := filepath.Dir(manifest)
		errs := Validate(os.DirFS(dir)).Categorize().Errors
		for _, f := range errs {
			t.Errorf(".claude/skills/%s: %s %s: %s", filepath.Base(dir), f.Code, f.Path, f.Message)
		}
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
