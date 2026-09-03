package skillpkg

import (
	"os"
	"path/filepath"
	"testing"
)

// The repository's own agent skills go through the product's own validator.
//
// Skill Hub exists to validate Agent Skills packages, and .claude/skills/ holds
// five of them that the coding agents working on this repo load. On 2026-09-04
// two of the five had no `name:` field — a Skill this validator rejects with
// `name-missing` — and nothing noticed, because the only reader was Claude Code,
// which derives the name from the directory. A repository that ships a Skill
// validator and cannot pass it over its own Skills is the cheapest possible
// dogfood to have skipped.
//
// Errors only. Warnings and infos (no licence, an external URL) are the normal
// state of an in-repo Skill and are not what this test is about.
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

// Walk up from the package directory until both AGENTS.md and .claude/ exist.
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
