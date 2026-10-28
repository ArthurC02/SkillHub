package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func evidenceRepo(t *testing.T) (gitRepo, string) {
	t.Helper()
	return evidenceRepoCiting(t, "src/a.py", "rule = 1\nother = 2\n", false)
}

func evidenceRepoCiting(t *testing.T, citedPath, content string, withoutHash bool) (gitRepo, string) {
	t.Helper()
	t.Setenv("PYTHONUTF8", "1")
	real, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	original := domainMemoryTool
	t.Cleanup(func() { domainMemoryTool = original })
	domainMemoryTool = func(_ string, args ...string) (string, error) { return original(real, args...) }

	repo := newGitRepo(t)
	repo.git(t, "config", "core.autocrlf", "true")
	writeAt(t, repo.root, citedPath, content)
	cited, err := domainMemoryTool(repo.root, "cite", "--repo-root", repo.root, "--path", citedPath, "--lines", "1-1")
	if err != nil {
		t.Fatalf("cite: %v\n%s", err, cited)
	}
	var citation map[string]any
	_, body, _ := strings.Cut(cited, "{")
	if err := json.Unmarshal([]byte("{"+body), &citation); err != nil {
		t.Fatalf("cite printed %q: %v", cited, err)
	}
	if withoutHash {
		delete(citation, "excerpt_sha256")
	}
	files := map[string]string{citedPath: content, runtimeDockerfile: ""}
	for path, text := range conceptRegistry(map[string]string{"rule": ruleConcept(citation, "S.")}) {
		files[path] = text
	}
	repo.commit(t, files)
	return repo, repo.head(t)
}

func ruleConcept(citation map[string]any, statement string) string {
	lines, _ := citation["lines"].(map[string]any)
	concept := fmt.Sprintf("---\ntype: Business Rule\nstatus: stable\nsources:\n  - resource: \"%s#L%v-%v\"\n",
		citation["path"], lines["start"], lines["end"])
	if digest, ok := citation["excerpt_sha256"].(string); ok {
		concept += fmt.Sprintf("    excerpt_sha256: \"%s\"\n", digest)
	}
	return concept + "---\n" + statement + "\n"
}

func conceptRegistry(rules map[string]string) map[string]string {
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	index := "# Business Rules\n\n"
	files := map[string]string{"docs/domain-memory/registry/index.md": "---\nokf_version: \"0.2\"\n---\n# Domain Registry\n\n- [Business Rules](/rules/index.md)\n"}
	for _, id := range ids {
		index += fmt.Sprintf("- [%s](/rules/%s)\n", id, id)
		files["docs/domain-memory/registry/rules/"+id+".md"] = rules[id]
	}
	files["docs/domain-memory/registry/rules/index.md"] = index
	return files
}

func pushProblems(t *testing.T, repo gitRepo, base string) []string {
	t.Helper()
	return rangeProblems(t, repo, base+"..HEAD")
}

func rangeProblems(t *testing.T, repo gitRepo, rangeSpec string) []string {
	t.Helper()
	problems, warnings, err := committedEvidenceProblems(repo.root, rangeSpec)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("the evidence check did not run: %v %v", err, warnings)
	}
	return problems
}

func TestPushingAnEditToACitedLineIsStoppedAsStale(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "rule = 3\nother = 2\n"})
	problems := pushProblems(t, repo, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/rule cites src/a.py:1-1, which is stale as committed") {
		t.Fatalf("an edit to a cited line passed the push: %v", problems)
	}
}

func TestPushingAnEditOutsideTheCitedLinesPasses(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "rule = 1\nother = 3\n"})
	if problems := pushProblems(t, repo, base); len(problems) != 0 {
		t.Fatalf("an edit outside the cited lines failed the push: %v", problems)
	}
}

func TestPushingTheRemovalOfACitedFileIsStoppedAsMissing(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.git(t, "rm", "-q", "--", "src/a.py")
	repo.git(t, "commit", "-q", "-m", "delete")
	problems := pushProblems(t, repo, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "which is missing as committed") {
		t.Fatalf("removing a cited file passed the push: %v", problems)
	}
}

func TestTheEvidenceCheckReadsTheCommittedBytesNotTheWorkingTree(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "rule = 1\nother = 3\n"})
	writeAt(t, repo.root, "src/a.py", "rule = 9\nother = 3\n")
	if problems := pushProblems(t, repo, base); len(problems) != 0 {
		t.Fatalf("someone else's working copy failed a clean push: %v", problems)
	}
}

func TestAnEvidenceToolThatCannotRunWarnsInsteadOfBlockingThePush(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "rule = 3\nother = 2\n"})
	domainMemoryTool = func(string, ...string) (string, error) { return "", errors.New("uv: not found") }
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "Domain Memory evidence skipped (uv: not found); CI still checks it") {
		t.Fatalf("a missing tool did not warn: %v %v %v", problems, warnings, err)
	}
}

func TestThePrePushHookStopsAStaleCitation(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "rule = 3\nother = 2\n"})
	var out strings.Builder
	pushed := "refs/heads/main " + repo.head(t) + " refs/heads/main " + base + "\n"
	if err := preflight(repo.root, []string{"--hook"}, strings.NewReader(pushed), &out); err == nil || !strings.Contains(out.String(), "which is stale as committed") {
		t.Fatalf("the hook passed a stale citation: %v\n%s", err, out.String())
	}
}

const currentReport = `{"results":[{"asset":"rules","id":"rule","path":"src/a.py","status":"current","cited_lines":{"start":1,"end":1}}]}`

func stubEvidenceTool(output string, err error) {
	export := domainMemoryTool
	domainMemoryTool = func(root string, args ...string) (string, error) {
		if args[0] == "verify-evidence" {
			return output, err
		}
		return export(root, args...)
	}
}

func editCitedLine(t *testing.T, repo gitRepo) {
	t.Helper()
	repo.commit(t, map[string]string{"src/a.py": "rule = 3\nother = 2\n"})
}

func TestAPushedCommitThatIsNotCheckedOutIsStoppedAsStale(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	pushed := repo.head(t)
	repo.git(t, "checkout", "-q", "--detach", base)
	problems := rangeProblems(t, repo, base+".."+pushed)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/rule cites src/a.py:1-1, which is stale as committed") {
		t.Fatalf("the pushed commit was not the one checked: %v", problems)
	}
}

func TestACitationAddedInAPushThatIsNotCheckedOutIsChecked(t *testing.T) {
	repo, base := evidenceRepo(t)
	writeAt(t, repo.root, "src/c.py", "fact = 1\n")
	cited, err := domainMemoryTool(repo.root, "cite", "--repo-root", repo.root, "--path", "src/c.py", "--lines", "1-1")
	if err != nil {
		t.Fatalf("cite: %v\n%s", err, cited)
	}
	var citation map[string]any
	_, body, _ := strings.Cut(cited, "{")
	if err := json.Unmarshal([]byte("{"+body), &citation); err != nil {
		t.Fatalf("cite printed %q: %v", cited, err)
	}
	files := map[string]string{"src/c.py": "fact = 1\n"}
	existing, err := os.ReadFile(filepath.Join(repo.root, "docs", "domain-memory", "registry", "rules", "rule.md"))
	if err != nil {
		t.Fatal(err)
	}
	for path, text := range conceptRegistry(map[string]string{"rule": string(existing), "later": ruleConcept(citation, "T.")}) {
		files[path] = text
	}
	repo.commit(t, files)
	repo.commit(t, map[string]string{"src/c.py": "fact = 2\n"})
	pushed := repo.head(t)
	repo.git(t, "checkout", "-q", "--detach", base)
	problems := rangeProblems(t, repo, base+".."+pushed)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/later cites src/c.py:1-1, which is stale as committed") {
		t.Fatalf("the citation the push added was not checked: %v", problems)
	}
}

func TestPushingTheRenameOfACitedFileIsStoppedAsMissing(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.git(t, "mv", "src/a.py", "src/b.py")
	repo.git(t, "commit", "-q", "-m", "rename")
	problems := pushProblems(t, repo, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/rule cites src/a.py, which is missing as committed") {
		t.Fatalf("renaming a cited file passed the push: %v", problems)
	}
}

func TestPushingAnEditToACitedLineInANonASCIIPathIsStoppedAsStale(t *testing.T) {
	repo, base := evidenceRepoCiting(t, "docs/文件.md", "規則 = 1\n其他 = 2\n", false)
	repo.commit(t, map[string]string{"docs/文件.md": "規則 = 3\n其他 = 2\n"})
	problems := pushProblems(t, repo, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/rule cites docs/文件.md:1-1, which is stale as committed") {
		t.Fatalf("an edit to a cited non-ASCII path passed the push: %v", problems)
	}
}

func TestPushingALineInsertedAboveACitedLinePasses(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"src/a.py": "new = 0\nrule = 1\nother = 2\n"})
	if problems := pushProblems(t, repo, base); len(problems) != 0 {
		t.Fatalf("a shifted citation failed the push: %v", problems)
	}
}

func TestPushingAnEditToAFileCitedWithoutAHashIsStoppedAsUnhashed(t *testing.T) {
	repo, base := evidenceRepoCiting(t, "src/a.py", "rule = 1\nother = 2\n", true)
	repo.commit(t, map[string]string{"src/a.py": "rule = 1\nother = 3\n"})
	problems := pushProblems(t, repo, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "rules/rule cites src/a.py:1-1, which is unhashed as committed") {
		t.Fatalf("an unhashed citation passed the push: %v", problems)
	}
}

func TestAnUnknownEvidenceStatusIsAProblem(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool(`{"results":[{"asset":"rules","id":"rule","path":"src/a.py","status":"unreadable","cited_lines":{"start":1,"end":1}}]}`, nil)
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	want := "Domain Memory evidence: rules/rule cites src/a.py:1-1, which is unreadable as committed; check the fact still holds, then re-cite it (verify-evidence --path src/a.py)"
	if err != nil || len(warnings) != 0 || len(problems) != 1 || problems[0] != want {
		t.Fatalf("an unknown status was not a problem: %v %v %v", problems, warnings, err)
	}
}

func TestAStaleReportFollowedByTheToolsSummaryLineIsAProblem(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool(`{"results":[{"asset":"rules","id":"rule","path":"src/a.py","status":"stale","cited_lines":{"start":1,"end":1}}]}`+"\nverify-evidence failed: 1 stale citation; run recite on each to cite its current lines", errors.New("exit status 1"))
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	want := "Domain Memory evidence: rules/rule cites src/a.py:1-1, which is stale as committed; check the fact still holds, then re-cite it (verify-evidence --path src/a.py)"
	if err != nil || len(warnings) != 0 || len(problems) != 1 || problems[0] != want {
		t.Fatalf("a stale report with a trailing summary was not a problem: %v %v %v", problems, warnings, err)
	}
}

func TestAReportWithoutResultsWarnsInsteadOfPassing(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool("{}", nil)
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 || len(warnings) != 1 || warnings[0] != "Domain Memory evidence skipped ({}); CI still checks it" {
		t.Fatalf("a report without results did not warn: %v %v %v", problems, warnings, err)
	}
}

func TestAToolThatFailsWithOnlyCurrentResultsIsAProblem(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool(currentReport, errors.New("exit status 1"))
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	want := "Domain Memory evidence: verify-evidence failed on the committed bytes (exit status 1); run it with --path on the files you changed"
	if err != nil || len(warnings) != 0 || len(problems) != 1 || problems[0] != want {
		t.Fatalf("a failing tool with only current results passed: %v %v %v", problems, warnings, err)
	}
}

func TestAToolThatPassesWithOnlyCurrentResultsIsClean(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool(currentReport, nil)
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	if err != nil || len(warnings) != 0 || len(problems) != 0 {
		t.Fatalf("a passing tool was not clean: %v %v %v", problems, warnings, err)
	}
}

func TestARangeWhoseEndHasNoRegistryHasNoProblemsAndNoWarnings(t *testing.T) {
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "a\n"})
	base := repo.head(t)
	repo.commit(t, map[string]string{"README.md": "b\n"})
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 || len(warnings) != 0 {
		t.Fatalf("a repository without a registry was checked: %v %v %v", problems, warnings, err)
	}
}

func TestARangeNamingAShaThatDoesNotExistIsAnError(t *testing.T) {
	repo, base := evidenceRepo(t)
	_, _, err := committedEvidenceProblems(repo.root, base+".."+strings.Repeat("1", 40))
	if err == nil || !strings.Contains(err.Error(), "ls-tree") {
		t.Fatalf("a git failure was read as no registry: %v", err)
	}
}

func TestARegistryFileThatIsNotAConceptIsNamedAndLeftToCI(t *testing.T) {
	repo, base := evidenceRepo(t)
	repo.commit(t, map[string]string{"docs/domain-memory/registry/rules/rule.md": "not a concept"})
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 0 || len(warnings) != 1 ||
		!strings.Contains(warnings[0], "Domain Memory evidence skipped (ERROR: rules/rule.md: a concept document starts with a --- frontmatter line") {
		t.Fatalf("an unparsable registry file was not named: %v %v %v", problems, warnings, err)
	}
}

func TestAWarningFromTheEvidenceToolIsPrintedByThePrePushHook(t *testing.T) {
	repo, base := evidenceRepo(t)
	editCitedLine(t, repo)
	stubEvidenceTool("", errors.New("uv: not found"))
	var out strings.Builder
	pushed := "refs/heads/main " + repo.head(t) + " refs/heads/main " + base + "\n"
	err := preflight(repo.root, []string{"--hook"}, strings.NewReader(pushed), &out)
	if err != nil || !strings.Contains(out.String(), "WARN Domain Memory evidence skipped (uv: not found); CI still checks it\n") {
		t.Fatalf("the hook did not print the warning: %v\n%s", err, out.String())
	}
}

func TestAnArchiveEntryThatLeavesTheSnapshotIsRefused(t *testing.T) {
	target := t.TempDir()
	err := writeArchived(target, "../x", strings.NewReader("y"))
	if err == nil || err.Error() != "git archive entry escapes the snapshot: ../x" {
		t.Fatalf("a path outside the snapshot was written: %v", err)
	}
}

func TestAnArchiveEntryInASiblingDirectoryWithTheSamePrefixIsRefused(t *testing.T) {
	target := t.TempDir()
	name := "../" + filepath.Base(target) + "x/f"
	if err := writeArchived(target, name, strings.NewReader("y")); err == nil {
		t.Fatalf("a sibling directory sharing the snapshot's name prefix was written: %s", name)
	}
}

func TestAnArchiveEntryInsideTheSnapshotIsWritten(t *testing.T) {
	target := t.TempDir()
	if err := writeArchived(target, "docs/a.txt", strings.NewReader("body")); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(target, "docs", "a.txt"))
	if err != nil || string(written) != "body" {
		t.Fatalf("the entry was not written: %q %v", written, err)
	}
}
