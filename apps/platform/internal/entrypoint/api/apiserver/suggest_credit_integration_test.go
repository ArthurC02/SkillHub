package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func suggestModel(t *testing.T, costUSD float64) (url string, calls *atomic.Int32) {
	t.Helper()
	calls = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/suggest-criteria" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(llmclient.SuggestCriteriaResponse{
			Criteria: []llmclient.SuggestedCriterion{{Text: "The summary names every column."}},
			Usage: &llmclient.GatewayUsage{
				PromptTokens: 90, CompletionTokens: 12, CostUSD: &costUSD, CostSource: llmclient.CostSourceGateway,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, calls
}

func creditBalance(t *testing.T, c *client) int64 {
	t.Helper()
	var balance int64
	if err := testPool.QueryRow(context.Background(),
		`SELECT balance_credits FROM credit_accounts WHERE user_id = $1`, mustUUID(t, c.userID)).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func suggestDebits(t *testing.T, c *client) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM cost_events
		WHERE kind = 'suggest_criteria' AND user_id = $1 AND usd_micros = 10000`, mustUUID(t, c.userID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSuggestingCriteriaBelowTheStartThresholdIsRefusedBeforeTheModelIsCalled(t *testing.T) {
	pool := requireDB(t)
	url, calls := suggestModel(t, 0.01)
	a := newAPIWithLLM(t, pool, url)
	a.startingCredits = 69
	alice := a.login(t, "suggest-credit-below")
	_, id := newTestCase(t, pool, a, alice, "suggest-below")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("suggest at 69 credits: got %d (%v), want 422", code, body)
	}
	if calls.Load() != 0 {
		t.Errorf("the model was called %d times for a refused suggestion", calls.Load())
	}
	if got := creditBalance(t, alice); got != 69 {
		t.Errorf("balance after a refused suggestion = %d, want 69", got)
	}
}

func TestSuggestingCriteriaAtTheStartThresholdIsDebitedByUsage(t *testing.T) {
	pool := requireDB(t)
	url, calls := suggestModel(t, 0.01)
	a := newAPIWithLLM(t, pool, url)
	a.startingCredits = 70
	alice := a.login(t, "suggest-credit-at")
	_, id := newTestCase(t, pool, a, alice, "suggest-at")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusOK {
		t.Fatalf("suggest at 70 credits: got %d (%v), want 200", code, body)
	}
	if calls.Load() != 1 {
		t.Errorf("model calls = %d, want 1", calls.Load())
	}
	if got := creditBalance(t, alice); got != 70-13 {
		t.Errorf("balance after a $0.01 suggestion = %d, want 57 (13 credits at the default markup)", got)
	}
	if n := suggestDebits(t, alice); n != 1 {
		t.Errorf("suggest_criteria cost events = %d, want 1", n)
	}
}
