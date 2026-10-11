package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releasedManifest = `{"name": "domain-memory", "version": "0.1.0"}`

func releasedPlugin(t *testing.T) gitRepo {
	t.Helper()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{
		".claude/skills/domain-memory/.claude-plugin/plugin.json": releasedManifest,
		".claude/skills/domain-memory/SKILL.md":                   "# Domain Memory\nRead first.\n",
		".claude/skills/domain-memory/scripts/registry.py":        "print('registry')\n",
		".claude/skills/domain-memory/scripts/contest.py":         "print('contest')\n",
		".claude/skills/domain-memory/scripts/test_registry.py":   "print('test')\n",
		".claude/skills/domain-memory/evals/scenarios.json":       "{}\n",
		"tools/devctl/go.mod":                                     "module devctl\n",
	})
	if err := recordDomainMemoryRelease(repo.root, io.Discard); err != nil {
		t.Fatalf("record the first release: %v", err)
	}
	return repo
}

func editPlugin(t *testing.T, repo gitRepo, relative, contents string) {
	t.Helper()
	writeTestFile(t, repo.root, ".claude/skills/domain-memory/"+relative, contents)
}

func expectOneProblemContaining(t *testing.T, problems []string, want string) {
	t.Helper()
	if len(problems) != 1 || !strings.Contains(problems[0], want) {
		t.Fatalf("problems = %q, want one containing %q", problems, want)
	}
}

func TestAnUnchangedDomainMemoryPluginReportsNothing(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	if problems := domainMemoryReleaseProblems(repo.root); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}

func TestADistributedFileChangedWithoutABumpNamesTheVersionToBump(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "SKILL.md", "# Domain Memory\nRead the Registry first.\n")
	expectOneProblemContaining(t, domainMemoryReleaseProblems(repo.root), "still says version 0.1.0; bump the version")
}

func TestAScriptWhoseNameOnlyContainsTestIsStillDistributed(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "scripts/contest.py", "print('changed')\n")
	expectOneProblemContaining(t, domainMemoryReleaseProblems(repo.root), "still says version 0.1.0")
}

func TestANewFileNotYetCommittedIsAChange(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "ruff.toml", "line-length = 100\n")
	expectOneProblemContaining(t, domainMemoryReleaseProblems(repo.root), "still says version 0.1.0")
}

func TestAnIgnoredFileIsNotAChange(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	writeTestFile(t, repo.root, ".gitignore", "__pycache__/\n")
	editPlugin(t, repo, "scripts/__pycache__/registry.cpython-310.pyc", "bytecode")
	if problems := domainMemoryReleaseProblems(repo.root); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}

func TestEvalsAndPythonTestsAreNotDistributed(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "evals/scenarios.json", `{"changed": true}`+"\n")
	editPlugin(t, repo, "scripts/test_registry.py", "print('changed')\n")
	if problems := domainMemoryReleaseProblems(repo.root); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}

func TestALineEndingOnlyDifferenceIsNotAChange(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "SKILL.md", "# Domain Memory\r\nRead first.\r\n")
	if problems := domainMemoryReleaseProblems(repo.root); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}

func TestABumpNotYetRecordedAsksForTheRecord(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "SKILL.md", "# Domain Memory\nRead the Registry first.\n")
	editPlugin(t, repo, ".claude-plugin/plugin.json", `{"name": "domain-memory", "version": "0.1.1"}`)
	expectOneProblemContaining(t, domainMemoryReleaseProblems(repo.root), "records 0.1.0 but the plugin is 0.1.1")
}

func TestAMissingReleaseRecordIsReported(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	if err := os.Remove(filepath.Join(repo.root, "tools", "devctl", "domain-memory-release.json")); err != nil {
		t.Fatal(err)
	}
	expectOneProblemContaining(t, domainMemoryReleaseProblems(repo.root), "domain-memory-release.json is missing")
}

func TestRecordingRefusesAContentChangeWithoutABumpAndLeavesTheRecord(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	recordPath := filepath.Join(repo.root, "tools", "devctl", "domain-memory-release.json")
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	editPlugin(t, repo, "SKILL.md", "# Domain Memory\nRead the Registry first.\n")
	err = recordDomainMemoryRelease(repo.root, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "still says version 0.1.0") {
		t.Fatalf("err = %v, want a refusal naming version 0.1.0", err)
	}
	after, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("record changed on refusal:\n%s\nwas\n%s", after, before)
	}
}

func TestRecordingAfterABumpClearsTheProblem(t *testing.T) {
	t.Parallel()
	repo := releasedPlugin(t)
	editPlugin(t, repo, "SKILL.md", "# Domain Memory\nRead the Registry first.\n")
	editPlugin(t, repo, ".claude-plugin/plugin.json", `{"name": "domain-memory", "version": "0.1.1"}`)
	if err := recordDomainMemoryRelease(repo.root, io.Discard); err != nil {
		t.Fatalf("record after bump: %v", err)
	}
	if problems := domainMemoryReleaseProblems(repo.root); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}

func TestARepositoryWithoutThePluginIsNotChecked(t *testing.T) {
	t.Parallel()
	if problems := domainMemoryReleaseProblems(t.TempDir()); len(problems) != 0 {
		t.Fatalf("problems = %q, want none", problems)
	}
}
