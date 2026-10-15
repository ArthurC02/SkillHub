package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func evidenceRepo(t *testing.T) (gitRepo, string) {
	t.Helper()
	real, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	original := domainMemoryTool
	t.Cleanup(func() { domainMemoryTool = original })
	domainMemoryTool = func(_ string, args ...string) (string, error) { return original(real, args...) }

	repo := newGitRepo(t)
	writeAt(t, repo.root, "src/a.py", "rule = 1\nother = 2\n")
	cited, err := domainMemoryTool(repo.root, "cite", "--repo-root", repo.root, "--path", "src/a.py", "--start", "1", "--end", "1")
	if err != nil {
		t.Fatalf("cite: %v\n%s", err, cited)
	}
	var citation map[string]any
	_, body, _ := strings.Cut(cited, "{")
	if err := json.Unmarshal([]byte("{"+body), &citation); err != nil {
		t.Fatalf("cite printed %q: %v", cited, err)
	}
	rules, err := json.MarshalIndent(map[string]any{"rules": []any{map[string]any{
		"id": "rule", "statement": "S.", "status": "reviewed", "evidence": []any{citation},
	}}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"src/a.py": "rule = 1\nother = 2\n", "docs/domain-memory/registry/rules.json": string(rules) + "\n", runtimeDockerfile: ""}
	for name, key := range map[string]string{
		"contexts": "contexts", "vocabulary": "terms", "aggregates": "aggregates", "contracts": "contracts",
		"interactions": "interactions", "decisions": "decisions", "events": "events", "capabilities": "capabilities",
		"value-objects": "value_objects", "dependency-policies": "dependencies",
	} {
		files["docs/domain-memory/registry/"+name+".json"] = `{"` + key + `": []}` + "\n"
	}
	repo.commit(t, files)
	return repo, repo.head(t)
}

func pushProblems(t *testing.T, repo gitRepo, base string) []string {
	t.Helper()
	problems, warnings, err := committedEvidenceProblems(repo.root, base+"..HEAD")
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
