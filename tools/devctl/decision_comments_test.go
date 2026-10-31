package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeJudge struct {
	server   *httptest.Server
	requests atomic.Int32
}

func newFakeJudge(t *testing.T, probability func(text string) float64, status int) (*fakeJudge, decisionJudge) {
	t.Helper()
	fake := &fakeJudge{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-key" || r.URL.Path != "/v1/systemone" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "down", status)
			return
		}
		var body struct {
			State struct {
				Comments []struct{ Text string } `json:"comments"`
			} `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		answers := map[string]map[string]float64{}
		for i, c := range body.State.Comments {
			answers["c"+strconv.Itoa(i)] = map[string]float64{"noul": probability(c.Text)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(fake.server.Close)
	return fake, decisionJudge{baseURL: fake.server.URL, key: "test-key", client: fake.server.Client()}
}

func decisionWhen(marker string) func(string) float64 {
	return func(text string) float64 {
		if strings.Contains(text, marker) {
			return decisionLogThreshold
		}
		return decisionLogThreshold - 0.01
	}
}

func decisionRepo(t *testing.T, files map[string]string) (gitRepo, string) {
	t.Helper()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"README.md": "x\n", runtimeDockerfile: "FROM scratch\n"})
	base := repo.head(t)
	repo.commit(t, files)
	return repo, base
}

func TestADecisionRecordAtTheThresholdIsReportedAndOneJustBelowIsNot(t *testing.T) {
	t.Parallel()
	repo, base := decisionRepo(t, map[string]string{"apps/x.go": "package x\n\n// Until last week this was decided by the owner.\nfunc A() {}\n\n// A walks the list once.\nfunc B() {}\n"})
	_, judge := newFakeJudge(t, decisionWhen("Until last week"), http.StatusOK)
	warnings, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", judge)
	if err != nil || len(warnings) != 1 || !strings.HasPrefix(warnings[0], "apps/x.go:3 reads like a decision record (0.45)") {
		t.Fatalf("want exactly the line-3 decision record reported: %v %v", warnings, err)
	}
}

func TestACommentAlreadyInTheBaseIsNotSentAgain(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{"apps/x.go": "package x\n\n// Until last week this was decided by the owner.\nfunc A() {}\n"})
	base := repo.head(t)
	repo.commit(t, map[string]string{"apps/x.go": "package x\n\n// Until last week this was decided by the owner.\nfunc A() {}\n\nfunc B() {}\n"})
	fake, judge := newFakeJudge(t, decisionWhen("Until"), http.StatusOK)
	warnings, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", judge)
	if err != nil || len(warnings) != 0 || fake.requests.Load() != 0 {
		t.Fatalf("an unchanged comment was judged again: %v %v requests=%d", warnings, err, fake.requests.Load())
	}
}

func TestTestFilesAndUncoveredFilesAreNotJudged(t *testing.T) {
	t.Parallel()
	repo, base := decisionRepo(t, map[string]string{
		"apps/x_test.go":         "package x\n\n// Until last week seq 2 was lost.\n",
		"apps/web/a.test.tsx":    "// Until last week B came back.\nexport {}\n",
		"apps/llm/tests/t.py":    "# Until last week it failed.\n",
		"docs/notes.go":          "package x\n\n// Until last week.\n",
		"apps/node_modules/m.ts": "// Until last week.\n",
	})
	fake, judge := newFakeJudge(t, decisionWhen("Until"), http.StatusOK)
	warnings, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", judge)
	if err != nil || len(warnings) != 0 || fake.requests.Load() != 0 {
		t.Fatalf("test or uncovered files were judged: %v %v requests=%d", warnings, err, fake.requests.Load())
	}
}

func TestWithoutAKeyNothingLeavesTheMachine(t *testing.T) {
	t.Parallel()
	repo, base := decisionRepo(t, map[string]string{"apps/x.go": "package x\n\n// Until last week.\n"})
	fake, judge := newFakeJudge(t, decisionWhen("Until"), http.StatusOK)
	warnings, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", decisionJudge{baseURL: judge.baseURL, client: judge.client})
	if err != nil || len(warnings) != 0 || fake.requests.Load() != 0 {
		t.Fatalf("the judge was called without a key: %v %v requests=%d", warnings, err, fake.requests.Load())
	}
}

func TestAnUnreachableJudgeWarnsOnceAndBlocksNothing(t *testing.T) {
	t.Parallel()
	repo, base := decisionRepo(t, map[string]string{"apps/x.go": "package x\n\n// Until last week.\n"})
	_, judge := newFakeJudge(t, decisionWhen("Until"), http.StatusServiceUnavailable)
	warnings, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", judge)
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "decision-comment advice skipped") {
		t.Fatalf("an unreachable judge did not degrade to one warning: %v %v", warnings, err)
	}
}

func TestCommentsAreSentInBatchesOfTen(t *testing.T) {
	t.Parallel()
	var src strings.Builder
	src.WriteString("package x\n")
	for i := range 21 {
		src.WriteString("\n// Comment number " + strconv.Itoa(i) + " walks the list.\nfunc F" + strconv.Itoa(i) + "() {}\n")
	}
	repo, base := decisionRepo(t, map[string]string{"apps/x.go": src.String()})
	fake, judge := newFakeJudge(t, decisionWhen("never"), http.StatusOK)
	if _, err := committedDecisionCommentWarnings(repo.root, base+"..HEAD", judge); err != nil {
		t.Fatal(err)
	}
	if got := fake.requests.Load(); got != 3 {
		t.Fatalf("21 comments took %d requests, want 3 batches of at most 10", got)
	}
}

func TestCommentBlocksJoinConsecutiveLinesAndSkipMachineMarkers(t *testing.T) {
	t.Parallel()
	src := "package x\n\n// first line\n// second line\nfunc A() {}\n//go:generate x\n// budget-over: x\n\n# not a go comment\n"
	got := commentBlocks("a.go", src, slashComments)
	if len(got) != 1 || got[0].line != 3 || got[0].text != "first line\nsecond line" {
		t.Fatalf("got %+v", got)
	}
}

func TestTheKeyComesFromTheEnvironmentBeforeDotEnv(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, ".env", "JEV_API_KEY=\"from-file\"\n")
	t.Setenv(decisionJudgeKeyName, "")
	if judge := newDecisionJudge(root); judge.key != "from-file" {
		t.Fatalf(".env key not read without its quotes: %q", judge.key)
	}
	t.Setenv(decisionJudgeKeyName, "from-env")
	if judge := newDecisionJudge(root); judge.key != "from-env" {
		t.Fatalf("environment did not win over .env: %q", judge.key)
	}
	if !newDecisionJudge(t.TempDir()).hasKey() {
		t.Fatal("environment key alone should enable the judge")
	}
	t.Setenv(decisionJudgeKeyName, "")
	if newDecisionJudge(t.TempDir()).hasKey() {
		t.Fatal("no key anywhere enabled the judge")
	}
}

func TestPreflightAdvisesOnADecisionRecordInThePushedRange(t *testing.T) {
	repo, base := decisionRepo(t, map[string]string{"apps/x.go": "package x\n\n// Until last week this was decided by the owner.\nfunc A() {}\n"})
	_, judge := newFakeJudge(t, decisionWhen("Until last week"), http.StatusOK)
	original := decisionJudgeBaseURL
	decisionJudgeBaseURL = judge.baseURL
	t.Cleanup(func() { decisionJudgeBaseURL = original })
	t.Setenv(decisionJudgeKeyName, "test-key")
	var out bytes.Buffer
	if _, err := committedProblems(repo.root, base+"..HEAD", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WARN apps/x.go:3 reads like a decision record") {
		t.Fatalf("preflight did not advise on the pushed decision record:\n%s", out.String())
	}
}
