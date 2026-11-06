package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type testCaseModel struct {
	mu    sync.Mutex
	asked []llmclient.SuggestTestCasesRequest
	reply []llmclient.SuggestedTestCase
}

func (m *testCaseModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.asked)
}

func suggestTestCasesModel(t *testing.T, reply ...llmclient.SuggestedTestCase) (string, *testCaseModel) {
	t.Helper()
	m := &testCaseModel{reply: reply}
	costUSD := 0.01
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/suggest-test-cases" {
			http.NotFound(w, r)
			return
		}
		var req llmclient.SuggestTestCasesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding the request Go sent: %v", err)
		}
		m.mu.Lock()
		m.asked = append(m.asked, req)
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(llmclient.SuggestTestCasesResponse{
			TestCases:     m.reply,
			PromptVersion: "suggest-test-cases/v1",
			Usage: &llmclient.GatewayUsage{
				PromptTokens: 90, CompletionTokens: 12, CostUSD: &costUSD, CostSource: llmclient.CostSourceGateway,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, m
}

func suggestTestCasesDebits(t *testing.T, c *client) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM cost_events
		WHERE kind = 'suggest_test_cases' AND user_id = $1 AND usd_micros = 10000`, mustUUID(t, c.userID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func suggestBody(skillID, focus string) string {
	return fmt.Sprintf(`{"skill_id":%q,"focus":%q}`, skillID, focus)
}

func TestSuggestingTestCasesBelowTheStartThresholdIsRefusedBeforeTheModelIsCalled(t *testing.T) {
	pool := requireDB(t)
	url, model := suggestTestCasesModel(t)
	a := newAPIWithLLM(t, pool, url)
	a.startingCredits = 69
	alice := a.login(t, "suggest-cases-below")
	skillID, _ := newTestCase(t, pool, a, alice, "suggest-cases-below")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/suggest", suggestBody(skillID, ""))
	if msg, _ := body["error"].(string); code != http.StatusUnprocessableEntity || !strings.Contains(msg, "建議測試題") {
		t.Fatalf("suggest at 69 credits: got %d (%v), want 422 naming test case suggestion", code, body)
	}
	if model.calls() != 0 {
		t.Errorf("the model was called %d times for a refused suggestion", model.calls())
	}
	if got := creditBalance(t, alice); got != 69 {
		t.Errorf("balance after a refused suggestion = %d, want 69", got)
	}
}

func TestSuggestingTestCasesAtTheStartThresholdIsDebitedByUsageAndStoresNothing(t *testing.T) {
	pool := requireDB(t)
	url, model := suggestTestCasesModel(t,
		llmclient.SuggestedTestCase{Name: "缺欄位的檔案", UserPrompt: "這份表少了金額欄", Criteria: []string{"指出缺少金額欄", " "}},
		llmclient.SuggestedTestCase{Name: "suggest-cases-at", UserPrompt: "和既有題目同名", Criteria: []string{"x"}},
	)
	a := newAPIWithLLM(t, pool, url)
	a.startingCredits = 70
	alice := a.login(t, "suggest-cases-at")
	skillID, _ := newTestCase(t, pool, a, alice, "suggest-cases-at")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/suggest", suggestBody(skillID, "  檔案缺欄位時  "))
	if code != http.StatusOK {
		t.Fatalf("suggest at 70 credits: got %d (%v), want 200", code, body)
	}
	got, _ := json.Marshal(body["suggestions"])
	if want := `[{"criteria":["指出缺少金額欄"],"name":"缺欄位的檔案","user_prompt":"這份表少了金額欄"}]`; string(got) != want {
		t.Errorf("suggestions = %s, want %s", got, want)
	}
	if model.calls() != 1 {
		t.Fatalf("model calls = %d, want 1", model.calls())
	}
	asked := model.asked[0]
	if asked.Focus != "檔案缺欄位時" || len(asked.ExistingNames) != 1 || asked.ExistingNames[0] != "suggest-cases-at" {
		t.Errorf("the model was asked with focus %q and existing names %v", asked.Focus, asked.ExistingNames)
	}
	if got := creditBalance(t, alice); got != 70-13 {
		t.Errorf("balance after a $0.01 suggestion = %d, want 57", got)
	}
	if n := suggestTestCasesDebits(t, alice); n != 1 {
		t.Errorf("suggest_test_cases cost events = %d, want 1", n)
	}
	var cases int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM test_cases WHERE skill_id = $1`, mustUUID(t, skillID)).Scan(&cases); err != nil {
		t.Fatal(err)
	}
	if cases != 1 {
		t.Errorf("test cases after a suggestion = %d, want the one that was there before", cases)
	}
}

func TestSuggestingTestCasesForAnotherWorkspacesSkillIsNotFoundAndCallsNoModel(t *testing.T) {
	pool := requireDB(t)
	url, model := suggestTestCasesModel(t)
	a := newAPIWithLLM(t, pool, url)
	alice := a.login(t, "suggest-cases-owner")
	bob := a.login(t, "suggest-cases-stranger")
	skillID, _ := newTestCase(t, pool, a, alice, "suggest-cases-owner")

	if code, body := bob.doJSON(t, http.MethodPost, "/test-cases/suggest", suggestBody(skillID, "")); code != http.StatusNotFound {
		t.Errorf("a stranger suggesting for alice's skill: got %d (%v), want 404", code, body)
	}
	if model.calls() != 0 {
		t.Errorf("the model was called %d times for a stranger", model.calls())
	}
}

func TestSuggestingTestCasesRefusesAFocusOnePastItsLimit(t *testing.T) {
	pool := requireDB(t)
	url, model := suggestTestCasesModel(t)
	a := newAPIWithLLM(t, pool, url)
	alice := a.login(t, "suggest-cases-focus")
	skillID, _ := newTestCase(t, pool, a, alice, "suggest-cases-focus")

	if code, body := alice.doJSON(t, http.MethodPost, "/test-cases/suggest",
		suggestBody(skillID, strings.Repeat("字", 501))); code != http.StatusBadRequest {
		t.Errorf("a 501-character focus: got %d (%v), want 400", code, body)
	}
	if code, body := alice.doJSON(t, http.MethodPost, "/test-cases/suggest",
		suggestBody(skillID, strings.Repeat("字", 500))); code != http.StatusOK {
		t.Errorf("a 500-character focus: got %d (%v), want 200", code, body)
	}
	if model.calls() != 1 {
		t.Errorf("model calls = %d, want 1 (only the focus within its limit)", model.calls())
	}
}

func TestSuggestingTestCasesUnderAP1HaltIsRefusedWithoutCallingTheModel(t *testing.T) {
	pool := requireDB(t)
	clearRunBacklog(t, pool)
	url, model := suggestTestCasesModel(t)
	a := newAPIWithLLM(t, pool, url)
	operator := a.login(t, freshName("operator-p1-test-cases"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	alice := a.login(t, "alice-p1-test-cases")
	skillID, _ := newTestCase(t, pool, a, alice, "p1-test-cases")

	declareP1Halt(t, operator, "model calls stop with dispatch")
	t.Cleanup(func() {
		if code, body := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
			`{"note":"test over"}`); code != http.StatusNoContent {
			t.Errorf("lifting the P1 halt: got %d (%v)", code, body)
		}
	})

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/suggest", suggestBody(skillID, ""))
	if msg, _ := body["error"].(string); code != http.StatusServiceUnavailable || !strings.Contains(msg, "測試題可以自己手動寫") {
		t.Errorf("suggesting test cases under a P1 halt: got %d %q, want 503 naming the manual path", code, msg)
	}
	if model.calls() != 0 {
		t.Errorf("the model was called %d times under a P1 halt", model.calls())
	}
}
