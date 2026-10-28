package main

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
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
	entry := ".githooks/pre-push"
	data, err := os.ReadFile(filepath.Join(root, entry))
	if err != nil {
		t.Fatal(err)
	}
	required := []string{entry}
	if !strings.Contains(string(data), "preflight --hook") {
		delegated := hooksReferencedBy(string(data), entry)
		if len(delegated) == 0 {
			t.Fatalf("%s neither runs devctl preflight --hook nor delegates to a hook that does:\n%s", entry, data)
		}
		reached := false
		for _, hook := range delegated {
			body, err := os.ReadFile(filepath.Join(root, hook))
			if err != nil {
				t.Fatalf("%s delegates to %s, which is missing: %v", entry, hook, err)
			}
			required = append(required, hook)
			if strings.Contains(string(body), "preflight --hook") {
				reached = true
			}
		}
		if !reached {
			t.Fatalf("no hook reached from %s runs devctl preflight --hook: %v", entry, delegated)
		}
	}
	for _, hook := range required {
		mode := strings.Fields(runIn(t, root, "git", "ls-files", "-s", "--", hook))
		if len(mode) == 0 || mode[0] != "100755" {
			t.Fatalf("%s is not executable in the index (%v); git on Linux and macOS will not run it", hook, mode)
		}
	}
}

func hooksReferencedBy(script, entry string) []string {
	split := func(r rune) bool { return r == '\'' || r == '"' || r == '\n' || r == ' ' || r == '\t' }
	besideEntry := strings.ReplaceAll(script, "$(git rev-parse --git-path hooks)", path.Dir(entry))
	var found []string
	for _, field := range strings.FieldsFunc(besideEntry, split) {
		if !strings.HasPrefix(field, ".githooks/") || field == entry {
			continue
		}
		if !slices.Contains(found, field) {
			found = append(found, field)
		}
	}
	return found
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

const overlongGoComment = "package x\n\n// one\n// two\n// three\n// four\nfunc F() {}\n"

func TestPreflightChecksTheCommentBudgetOfTheCommittedBytes(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n", runtimeDockerfile: ""})
	base := repo.head(t)

	repo.commit(t, map[string]string{"apps/platform/x/doc.go": "package x\n"})
	writeAt(t, repo.root, "apps/platform/x/doc.go", overlongGoComment)
	repo.git(t, "add", "--", "apps/platform/x/doc.go")
	if problems, err := committedCommentProblems(repo.root, base+"..HEAD"); err != nil || len(problems) != 0 {
		t.Fatalf("someone else's working copy failed a clean commit: %v %v", problems, err)
	}

	repo.commit(t, map[string]string{"apps/platform/x/doc.go": overlongGoComment, "docs/x.go": overlongGoComment})
	var out strings.Builder
	pushed := "refs/heads/main " + repo.head(t) + " refs/heads/main " + base + "\n"
	err := preflight(repo.root, []string{"--hook"}, strings.NewReader(pushed), &out)
	if err == nil || strings.Count(out.String(), "comment-budget:") != 1 ||
		!strings.Contains(out.String(), "comment-budget: apps/platform/x/doc.go has 1 comments to remove") {
		t.Fatalf("an over-long committed comment passed the push, or a file outside the budget was checked: %v\n%s", err, out.String())
	}
}

func TestACommittedFileIsReadWhenItsRevisionPathOutgrowsTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	const rootLength, fileLength, windowsPathLimit = 160, 75, 260
	root := t.TempDir()
	root = filepath.Join(root, strings.Repeat("d", rootLength-len(root)-1))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := gitRepo{root: root}
	repo.git(t, "init", "-q", "-b", "main")
	repo.commit(t, map[string]string{"README.md": "x\n"})
	base := repo.head(t)
	file := "apps/platform/" + strings.Repeat("f", fileLength-len("apps/platform/.go")) + ".go"
	repo.commit(t, map[string]string{file: overlongGoComment})
	pushed := repo.head(t)
	if len(root)+1+len(file) >= windowsPathLimit || len(root)+1+len(pushed)+1+len(file) <= windowsPathLimit {
		t.Fatalf("the fixture must fit a file under the limit and its revision path over it")
	}
	problems, err := committedCommentProblems(repo.root, base+".."+pushed)
	if err != nil || len(problems) != 1 {
		t.Fatalf("problems = %q, err = %v; want the one over-long comment found", problems, err)
	}
}

func TestCommentBudgetCoversExactlyTheFilesTheTreeScanReads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := []string{
		"x.go", "apps/platform/x.go", "apps/web/node_modules/p/x.go", "apps/.cache/x.go",
		"tools/devctl/testdata/x.go", ".github/workflows/x.go", ".github/x.go", "docs/x.go", "packages/a/gen/x.go",
	}
	var covered []string
	for _, file := range files {
		writeAt(t, root, file, overlongGoComment)
		if commentBudgetCovers(file) {
			covered = append(covered, file)
		}
	}
	violations, err := commentViolationsByFile(root)
	if err != nil {
		t.Fatal(err)
	}
	var scanned []string
	for file := range violations {
		scanned = append(scanned, file)
	}
	sort.Strings(covered)
	sort.Strings(scanned)
	if want := ".github/workflows/x.go apps/platform/x.go x.go"; strings.Join(covered, " ") != want {
		t.Fatalf("commentBudgetCovers %v, want %s", covered, want)
	}
	if strings.Join(covered, " ") != strings.Join(scanned, " ") {
		t.Fatalf("commentBudgetCovers %v, the tree scan read %v", covered, scanned)
	}
}

func TestFormatChecksThePushedCommitWhenItIsNotCheckedOut(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Fatal("gofmt ships with Go and is required for this test")
	}
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n"})
	base := repo.head(t)
	repo.commit(t, map[string]string{"apps/platform/x.go": unformattedGo})
	pushed := repo.head(t)
	repo.git(t, "checkout", "-q", "--detach", base)
	problems, _, err := formatProblems(repo.root, base+".."+pushed)
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "apps/platform/x.go is not gofmt-formatted") {
		t.Fatalf("the pushed commit was not the one checked: %v %v", problems, err)
	}
}

func TestCommentBudgetChecksThePushedCommitWhenItIsNotCheckedOut(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n"})
	base := repo.head(t)
	repo.commit(t, map[string]string{"apps/platform/x/doc.go": overlongGoComment})
	pushed := repo.head(t)
	repo.git(t, "checkout", "-q", "--detach", base)
	problems, err := committedCommentProblems(repo.root, base+".."+pushed)
	want := "comment-budget: apps/platform/x/doc.go has 1 comments to remove; see `go -C tools/devctl run . comment-lint apps/platform/x/doc.go` and AGENTS.md 慣例"
	if err != nil || len(problems) != 1 || problems[0] != want {
		t.Fatalf("the pushed commit was not the one checked: %v %v", problems, err)
	}
}

func TestCommentBudgetSkipsFilesDeletedInTheRange(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"apps/platform/x/doc.go": overlongGoComment})
	base := repo.head(t)
	repo.git(t, "rm", "-q", "--", "apps/platform/x/doc.go")
	repo.git(t, "commit", "-q", "-m", "delete")
	problems, err := committedCommentProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 {
		t.Fatalf("a deleted file was checked: %v %v", problems, err)
	}
}

func TestAnUnavailableFormatterWarnsOnceAndBlocksNothing(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n"})
	base := repo.head(t)
	repo.commit(t, map[string]string{"apps/web/src/a.ts": "export const a=1\n", "apps/web/src/b.ts": "export const b=2\n"})
	problems, warnings, err := formatProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 || len(warnings) != 1 || !strings.HasPrefix(warnings[0], "prettier skipped (formatter unavailable: ") {
		t.Fatalf("an unavailable formatter did not warn exactly once: %v %v %v", problems, warnings, err)
	}
}
