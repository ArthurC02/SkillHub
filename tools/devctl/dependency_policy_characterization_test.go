package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func dependencyPolicyFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, ".github/dependabot.yml", "updates:\n  - directories:\n      - /apps/web\n      - /apps/llm\n      - /apps/x\n")
	for name, body := range files {
		writeAt(t, root, name, body)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

func TestDependencyPolicyRequiresTheLockfileGuardSettingNextToEachLockfile(t *testing.T) {
	t.Parallel()
	const npmProblem = "apps/web/.npmrc must set ignore-scripts=true"
	const uvProblem = "apps/llm/pyproject.toml must set [tool.uv] exclude-newer"
	cases := []struct {
		name    string
		files   map[string]string
		problem string
		wanted  bool
	}{
		{"an npm lockfile whose .npmrc turns scripts off", map[string]string{
			"apps/web/package-lock.json": "{}\n", "apps/web/.npmrc": "ignore-scripts=true\n"}, npmProblem, false},
		{"an npm lockfile whose .npmrc leaves scripts on", map[string]string{
			"apps/web/package-lock.json": "{}\n", "apps/web/.npmrc": "audit=false\n"}, npmProblem, true},
		{"an npm lockfile with no .npmrc", map[string]string{
			"apps/web/package-lock.json": "{}\n"}, npmProblem, true},
		{"a uv lockfile whose pyproject sets a cooldown", map[string]string{
			"apps/llm/uv.lock": "version = 1\n", "apps/llm/pyproject.toml": "[tool.uv]\nexclude-newer = \"7 days\"\n"}, uvProblem, false},
		{"a uv lockfile whose pyproject sets no cooldown", map[string]string{
			"apps/llm/uv.lock": "version = 1\n", "apps/llm/pyproject.toml": "[project]\nname = \"x\"\n"}, uvProblem, true},
		{"a uv lockfile with no pyproject", map[string]string{
			"apps/llm/uv.lock": "version = 1\n"}, uvProblem, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			problems := strings.Join(dependencyPolicyProblems(dependencyPolicyFixture(t, c.files)), "\n")
			if got := strings.Contains(problems, c.problem); got != c.wanted {
				t.Fatalf("reported %q = %v, want %v; problems:\n%s", c.problem, got, c.wanted, problems)
			}
		})
	}
}

func TestDependencyPolicyReportsATrackedFileItCannotRead(t *testing.T) {
	t.Parallel()
	root := dependencyPolicyFixture(t, map[string]string{"apps/x/Dockerfile": "FROM scratch\n"})
	gone := filepath.Join(root, "apps", "x", "Dockerfile")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	problems := dependencyPolicyProblems(root)
	for _, problem := range problems {
		if strings.Contains(problem, gone) {
			return
		}
	}
	t.Fatalf("no problem names the unreadable %s; problems:\n%s", gone, strings.Join(problems, "\n"))
}
