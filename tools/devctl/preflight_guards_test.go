package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	passingGuardsTests = []string{"git", "--version"}
	failingGuardsTests = []string{"git", "skillhub-no-such-subcommand"}
	missingGuardsTests = []string{"skillhub-no-such-test-runner"}
)

func guardsRepo(t *testing.T) (gitRepo, string) {
	t.Helper()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n", guardsPluginDir + "/hooks/register.ts": "export {}\n"})
	return repo, repo.head(t)
}

func TestGuardsTestsDoNotRunWhenThePushLeavesThePluginAlone(t *testing.T) {
	t.Parallel()
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{"README.md": "y\n"})
	problems, warnings, err := committedGuardsTestProblems(repo.root, base+"..HEAD", failingGuardsTests)
	if err != nil || len(problems) != 0 || len(warnings) != 0 {
		t.Fatalf("a push outside the plugin ran its tests: %v %v %v", problems, warnings, err)
	}
}

func TestAFailingGuardsTestBlocksThePushThatChangedThePlugin(t *testing.T) {
	t.Parallel()
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{guardsPluginDir + "/hooks/register.ts": "export const x = 1\n"})
	problems, warnings, err := committedGuardsTestProblems(repo.root, base+"..HEAD", failingGuardsTests)
	if err != nil || len(warnings) != 0 || len(problems) != 1 || !strings.Contains(problems[0], "skillhub-no-such-subcommand") {
		t.Fatalf("a failing plugin test did not block the push: %v %v %v", problems, warnings, err)
	}
}

func TestPassingGuardsTestsLetThePushThrough(t *testing.T) {
	t.Parallel()
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{guardsPluginDir + "/tests/new.test.ts": "export {}\n"})
	problems, warnings, err := committedGuardsTestProblems(repo.root, base+"..HEAD", passingGuardsTests)
	if err != nil || len(problems) != 0 || len(warnings) != 0 {
		t.Fatalf("passing plugin tests blocked the push: %v %v %v", problems, warnings, err)
	}
}

func TestAMissingTestRunnerWarnsInsteadOfBlocking(t *testing.T) {
	t.Parallel()
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{guardsPluginDir + "/hooks/register.ts": "export const x = 1\n"})
	problems, warnings, err := committedGuardsTestProblems(repo.root, base+"..HEAD", missingGuardsTests)
	if err != nil || len(problems) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "did not run") {
		t.Fatalf("a missing test runner was not a warning: %v %v %v", problems, warnings, err)
	}
}

func TestGuardsTestsWarnWhenTheyRunOnAWorkingTreeThatIsNotThePushedCommit(t *testing.T) {
	t.Parallel()
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{guardsPluginDir + "/hooks/register.ts": "export const x = 1\n"})
	writeAt(t, repo.root, guardsPluginDir+"/hooks/register.ts", "export const x = 2\n")
	problems, warnings, err := committedGuardsTestProblems(repo.root, base+"..HEAD", passingGuardsTests)
	if err != nil || len(problems) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "ran on the working tree") {
		t.Fatalf("a working tree that differs from the pushed commit went unmentioned: %v %v %v", problems, warnings, err)
	}
}

func TestLastLinesKeepsTheTailOfALongReport(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ text, want string }{
		{"a\nb\nc\n", "b\nc"},
		{"c", "c"},
		{"b\nc", "b\nc"},
	} {
		if got := lastLines(tc.text, 2); got != tc.want {
			t.Fatalf("lastLines(%q, 2) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestPreflightRunsTheGuardsStepOnThePushedRange(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git is required for this test")
	}
	repo, base := guardsRepo(t)
	repo.commit(t, map[string]string{
		guardsPluginDir + "/hooks/register.ts": "export const x = 1\n",
		runtimeDockerfile:                      "FROM scratch\n",
	})
	t.Setenv("PATH", filepath.Dir(git))
	if commandInstalled(guardsTestCommand[0]) {
		t.Skipf("%s sits beside git, so PATH cannot hide it", guardsTestCommand[0])
	}
	var out bytes.Buffer
	if _, err := committedProblems(repo.root, base+"..HEAD", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), guardsPluginDir+" changed but") {
		t.Fatalf("preflight skipped the guards step on a push that changed the plugin:\n%s", out.String())
	}
}
