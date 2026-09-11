package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type gitRepo struct{ root string }

func newGitRepo(t *testing.T) gitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is required for this test")
	}
	repo := gitRepo{root: t.TempDir()}
	repo.git(t, "init", "-q", "-b", "main")
	return repo
}

func (r gitRepo) git(t *testing.T, args ...string) string {
	t.Helper()
	full := append([]string{"-C", r.root, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "core.autocrlf=false"}, args...)
	output, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (r gitRepo) commit(t *testing.T, files map[string]string) {
	t.Helper()
	names := make([]string, 0, len(files))
	for name, body := range files {
		writeAt(t, r.root, name, body)
		names = append(names, name)
	}
	sort.Strings(names)
	r.git(t, append([]string{"add", "--"}, names...)...)
	r.git(t, "commit", "-q", "-m", "change")
}

func (r gitRepo) head(t *testing.T) string {
	t.Helper()
	return r.git(t, "rev-parse", "HEAD")
}

const formattedGo = "package x\n\nfunc F() int {\n\treturn 1\n}\n"
const unformattedGo = "package x\nfunc F() int { return 1 }\n"

func TestFormatChecksTheCommittedBytesNotTheWorkingTree(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Fatal("gofmt ships with Go and is required for this test")
	}
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n"})
	base := repo.head(t)

	repo.commit(t, map[string]string{"apps/platform/x.go": formattedGo})
	writeAt(t, repo.root, "apps/platform/x.go", unformattedGo)
	if problems, warnings, err := formatProblems(repo.root, base+"..HEAD"); err != nil || len(problems) != 0 {
		t.Fatalf("someone else's unformatted working copy failed a clean commit: %v %v %v", problems, warnings, err)
	}

	repo.commit(t, map[string]string{"apps/platform/x.go": unformattedGo})
	problems, _, err := formatProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "apps/platform/x.go is not gofmt-formatted") {
		t.Fatalf("an unformatted commit passed: %v %v", problems, err)
	}
}

func TestFormatSkipsFilesDeletedInTheRange(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"apps/platform/x.go": unformattedGo})
	base := repo.head(t)
	repo.git(t, "rm", "-q", "--", "apps/platform/x.go")
	repo.git(t, "commit", "-q", "-m", "delete")
	if problems, _, err := formatProblems(repo.root, base+"..HEAD"); err != nil || len(problems) != 0 {
		t.Fatalf("a deleted file was checked: %v %v", problems, err)
	}
}

func TestAMissingFormatterWarnsInsteadOfBlockingThePush(t *testing.T) {
	t.Parallel()
	f := formatter{name: "ghost", cmd: "skillhub-no-such-formatter"}
	if _, err := f.format(t.TempDir(), "x"); err == nil || !strings.Contains(err.Error(), errFormatterUnavailable.Error()) {
		t.Fatalf("a missing formatter was not reported as unavailable: %v", err)
	}
	missingDependency := formatter{name: "prettier", cmd: "git", requires: filepath.Join(t.TempDir(), "prettier.cjs")}
	if _, err := missingDependency.format(t.TempDir(), "x"); err == nil || !strings.Contains(err.Error(), "run task bootstrap") {
		t.Fatalf("a formatter whose package is not installed was not reported as unavailable: %v", err)
	}
}

func TestFormatterFollowsWhatCIFormatChecks(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"apps/platform/internal/x.go": "gofmt",
		"apps/sandbox/cmd/x.go":       "gofmt",
		"tools/devctl/x.go":           "",
		"apps/web/src/App.tsx":        "prettier",
		"apps/web/index.html":         "prettier",
		"apps/llm/src/app.py":         "ruff",
		"apps/llm/pyproject.toml":     "",
		"docs/plans/04.md":            "",
	}
	for file, want := range cases {
		f, ok := formatterFor("/repo", file)
		if got := map[bool]string{true: f.name, false: ""}[ok]; got != want {
			t.Errorf("formatterFor(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestPrePushRangesFollowGitsHookInput(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"a.txt": "a\n"})
	remote := repo.head(t)
	repo.commit(t, map[string]string{"a.txt": "b\n"})
	local := repo.head(t)
	zero := strings.Repeat("0", 40)

	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"an update", "refs/heads/main " + local + " refs/heads/main " + remote + "\n", []string{remote + ".." + local}},
		{"a new branch", "refs/heads/x " + local + " refs/heads/x " + zero + "\n", []string{"origin/main.." + local}},
		{"a deletion", "(delete) " + zero + " refs/heads/x " + remote + "\n", nil},
		{"nothing to push", "", nil},
	}
	for _, tc := range cases {
		got, err := prePushRanges(repo.root, strings.NewReader(tc.input))
		if err != nil || strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%s: got %v %v, want %v", tc.name, got, err, tc.want)
		}
	}

	if _, err := prePushRanges(repo.root, strings.NewReader("refs/heads/main "+local+"\n")); err == nil {
		t.Error("a malformed hook line was accepted")
	}
	unknown := strings.Repeat("1", 40)
	_, err := prePushRanges(repo.root, strings.NewReader("refs/heads/main "+local+" refs/heads/main "+unknown+"\n"))
	if err == nil || !strings.Contains(err.Error(), "fetch first") {
		t.Errorf("a remote tip this clone lacks did not ask for a fetch: %v", err)
	}
}

func TestTheRepoHookRunsPreflight(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".githooks", "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "preflight --hook") {
		t.Fatalf(".githooks/pre-push does not run devctl preflight --hook:\n%s", data)
	}
	mode := strings.Fields(runIn(t, root, "git", "ls-files", "-s", "--", ".githooks/pre-push"))
	if len(mode) == 0 || mode[0] != "100755" {
		t.Fatalf(".githooks/pre-push is not executable in the index (%v); git on Linux and macOS will not run it", mode)
	}
}

func runIn(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(output)
}
