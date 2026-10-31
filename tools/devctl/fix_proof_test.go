package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixProofFake struct {
	messages []string
	judge    decisionJudge
}

func newFixProofFake(t *testing.T, probability float64, status int) *fixProofFake {
	t.Helper()
	fake := &fixProofFake{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State struct {
				Message string `json:"message"`
			} `json:"state"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		fake.messages = append(fake.messages, body.State.Message)
		if status != http.StatusOK {
			http.Error(w, "down", status)
			return
		}
		answers := map[string]map[string]float64{}
		for id := range body.Questions {
			answers[id] = map[string]float64{"noul": probability}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(server.Close)
	fake.judge = decisionJudge{baseURL: server.URL, key: "test-key", client: server.Client()}
	return fake
}

const unprovenFix = "fix(web): keep the draft\n\nThe draft was lost on reload; it is now kept.\n"

func TestAFixWithProofJustBelowTheThresholdIsWarnedAndOneAtItIsNot(t *testing.T) {
	t.Parallel()
	below := newFixProofFake(t, fixProofThreshold-0.01, http.StatusOK)
	if warning := fixProofWarning(unprovenFix, below.judge); !strings.HasPrefix(warning, "this fix commit reports no test seen failing without the change (0.34)") {
		t.Fatalf("a fix judged just below the threshold was not warned: %q", warning)
	}
	at := newFixProofFake(t, fixProofThreshold, http.StatusOK)
	if warning := fixProofWarning(unprovenFix, at.judge); warning != "" {
		t.Fatalf("a fix judged at the threshold was warned: %q", warning)
	}
}

func TestOnlyAFixWithoutVisibleProofIsSentToTheJudge(t *testing.T) {
	t.Parallel()
	for name, message := range map[string]string{
		"feature subject":      "feat(web): keep the draft\n\nNo proof here.\n",
		"subject mentions fix": "docs: explain the fix(web) commits\n",
		"reverted proof":       "fix(web): keep the draft\n\nWith the fix reverted the new test fails.\n",
		"mutant proof":         "fix: keep the draft\n\nA mutant turns the test red.\n",
		"chinese proof":        "fix: keep the draft\n\n還原修正後測試變紅。\n",
	} {
		fake := newFixProofFake(t, 0, http.StatusOK)
		if warning := fixProofWarning(message, fake.judge); warning != "" || len(fake.messages) != 0 {
			t.Errorf("%s: warning %q after %d requests", name, warning, len(fake.messages))
		}
	}
	for _, subject := range []string{"fix(web): a", "fix: a", "fix!: a"} {
		fake := newFixProofFake(t, 0, http.StatusOK)
		if fixProofWarning(subject+"\n", fake.judge) == "" || len(fake.messages) != 1 {
			t.Errorf("%q was not judged", subject)
		}
	}
}

func TestWithoutAKeyNoCommitMessageLeavesTheMachine(t *testing.T) {
	t.Parallel()
	fake := newFixProofFake(t, 0, http.StatusOK)
	keyless := decisionJudge{baseURL: fake.judge.baseURL, client: fake.judge.client}
	if warning := fixProofWarning(unprovenFix, keyless); warning != "" || len(fake.messages) != 0 {
		t.Fatalf("the judge was called without a key: %q requests=%d", warning, len(fake.messages))
	}
}

func TestAnUnreachableJudgeLeavesOneSkippedWarning(t *testing.T) {
	t.Parallel()
	fake := newFixProofFake(t, 0, http.StatusServiceUnavailable)
	if warning := fixProofWarning(unprovenFix, fake.judge); !strings.HasPrefix(warning, "fix-proof advice skipped (judge answered 503") {
		t.Fatalf("an unreachable judge did not degrade to one warning: %q", warning)
	}
}

func TestGitsCommentLinesAndTheDiffBelowTheScissorsAreNotSent(t *testing.T) {
	t.Parallel()
	fake := newFixProofFake(t, 1, http.StatusOK)
	raw := unprovenFix + "# Please enter the commit message.\n# ------------------------ >8 ------------------------\ndiff --git a/secret b/secret\n+token\n"
	fixProofWarning(raw, fake.judge)
	if len(fake.messages) != 1 || fake.messages[0] != strings.TrimSpace(unprovenFix) {
		t.Fatalf("sent %q, want only the message", fake.messages)
	}
}

func TestTheRepoCommitMsgHookRunsFixProofWithoutBlocking(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	hook := ".githooks/commit-msg"
	data, err := os.ReadFile(filepath.Join(root, hook))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `fix-proof "$1" || true`) {
		t.Fatalf("%s does not run devctl fix-proof on the message file without blocking the commit:\n%s", hook, data)
	}
	mode := strings.Fields(runIn(t, root, "git", "ls-files", "-s", "--", hook))
	if len(mode) == 0 || mode[0] != "100755" {
		t.Fatalf("%s is not executable in the index (%v); git on Linux and macOS will not run it", hook, mode)
	}
}

func TestFixProofReadsAMessageFileRelativeToTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "COMMIT_EDITMSG"), []byte(unprovenFix), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := newFixProofFake(t, 0, http.StatusOK)
	original := decisionJudgeBaseURL
	decisionJudgeBaseURL = fake.judge.baseURL
	t.Cleanup(func() { decisionJudgeBaseURL = original })
	t.Setenv(decisionJudgeKeyName, "test-key")
	var out bytes.Buffer
	if err := fixProof(root, []string{".git/COMMIT_EDITMSG"}, &out); err != nil || !strings.Contains(out.String(), "WARN this fix commit reports no test") {
		t.Fatalf("fix-proof did not warn on the message file: %v %q", err, out.String())
	}
}
