package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

type intentPythonCase struct {
	name, finish, status, reason  string
	refuseIntent, refuseEmbedding bool
}

type intentPythonRun struct {
	python string
	pool   *pgxpool.Pool
	query  string
	tc     intentPythonCase
}

type intentGatewayCalls struct {
	analyses, embeddings atomic.Int32
}

type intentGatewayRequest struct {
	Model    string            `json:"model"`
	Input    []string          `json:"input"`
	Metadata map[string]string `json:"metadata"`
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

func TestAnonymousSearchTraversesGoPythonAndGateway(t *testing.T) {
	python := creationPythonExecutable(t)
	t.Setenv("LLM_SERVICE_TOKEN", "test-service")
	t.Setenv("INTENT_MODEL", "gpt-6-luna")
	pool := requireDB(t)
	const query = "請將收支資料轉成報告"
	for _, tc := range []intentPythonCase{
		{name: "complete analysis", finish: "stop", status: "analyzed"},
		{name: "truncated analysis keeps vector search", finish: "length", status: "fallback", reason: "invalid_response"},
		{name: "gateway refusal keeps vector search", refuseIntent: true, status: "fallback", reason: "unavailable"},
		{name: "both capabilities unavailable disclose degradation", refuseIntent: true, refuseEmbedding: true, status: "fallback", reason: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runIntentPythonCase(t, intentPythonRun{python: python, pool: pool, query: query, tc: tc})
		})
	}
}

func runIntentPythonCase(t *testing.T, r intentPythonRun) {
	tc, pool := r.tc, r.pool
	model := uniqueWorklistLabel("intent-python-cost")
	t.Setenv("INTENT_MODEL", model)
	var calls intentGatewayCalls
	gateway := httptest.NewServer(intentPythonGateway(t, r, &calls))
	t.Cleanup(gateway.Close)
	a := newAPIWithLLM(t, pool, startCreationPython(t, r.python, gateway.URL))
	owner := a.login(t, uniqueWorklistLabel("intent-python"))
	markCatalog(t, pool, owner.workspaceID)
	unmarkCatalogOnCleanup(t, pool, owner.workspaceID)
	target := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("ledger"))
	seedEmbedding(t, pool, target, 1486)
	res, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape(r.query))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body catalogSearchBody
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || body.Interpretation.Status != tc.status || body.Interpretation.FallbackReason != tc.reason || body.Degraded != tc.refuseEmbedding {
		t.Fatalf("status=%d response=%+v", res.StatusCode, body)
	}
	if calls.analyses.Load() != 1 || calls.embeddings.Load() != 1 {
		t.Fatalf("calls: intent=%d embedding=%d; want one each, no retry", calls.analyses.Load(), calls.embeddings.Load())
	}
	wantCount := int64(1)
	if tc.refuseIntent {
		wantCount = 0
	}
	assertIntentCostRows(t, pool, model, wantCount)
	if found := searchResultsContain(body.Results, target); found == tc.refuseEmbedding {
		t.Fatalf("vector-only target present=%v, embedding unavailable=%v", found, tc.refuseEmbedding)
	}
	if tc.status == "analyzed" && !analyzedLedgerInterpretation(body.Interpretation) {
		t.Fatalf("interpretation=%+v", body.Interpretation)
	}
}

func intentPythonGateway(t *testing.T, r intentPythonRun, calls *intentGatewayCalls) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.Header.Get("Authorization") != "Bearer test-service-key" {
			t.Error("unexpected gateway method or credential")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request intentGatewayRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-response-cost", "0.001")
		switch {
		case req.URL.Path == "/v1/embeddings":
			calls.embeddings.Add(1)
			answerIntentEmbedding(t, w, r, request)
		case req.URL.Path == "/v1/chat/completions" && request.Metadata["operation"] == "analyze-intent":
			calls.analyses.Add(1)
			answerIntentAnalysis(t, w, r, request)
		default:
			http.Error(w, `{"error":{"message":"optional capability unavailable","type":"server_error"}}`, http.StatusServiceUnavailable)
		}
	}
}

func answerIntentEmbedding(t *testing.T, w http.ResponseWriter, r intentPythonRun, request intentGatewayRequest) {
	if len(request.Input) != 1 || request.Input[0] != r.query {
		t.Errorf("embedding input = %v, want original task", request.Input)
	}
	if r.tc.refuseEmbedding {
		http.Error(w, `{"error":{"message":"unavailable","type":"server_error"}}`, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"object": "list", "model": "text-embedding-3-small", "data": []map[string]any{{"object": "embedding", "index": 0, "embedding": unitVector(1486)}}, "usage": map[string]int{"prompt_tokens": 10, "total_tokens": 10}})
}

func answerIntentAnalysis(t *testing.T, w http.ResponseWriter, r intentPythonRun, request intentGatewayRequest) {
	if !intentRequestKeepsProvenance(request, r.query) {
		t.Error("intent request lost the task or prompt provenance")
	}
	if r.tc.refuseIntent {
		http.Error(w, `{"error":{"message":"budget exhausted","type":"budget_exceeded"}}`, http.StatusTooManyRequests)
		return
	}
	content := `{"intent":{"input":"收支資料","output":"報告","tools":null,"data":null,"environment":null},"keywords":["ledger"],"filters":{"script":null,"validation":null,"agent":null,"tier":null,"category":null}}`
	writeJSON(w, map[string]any{"id": "chatcmpl-intent", "object": "chat.completion", "created": 1, "model": request.Model, "choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": r.tc.finish}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
}

func intentRequestKeepsProvenance(request intentGatewayRequest, query string) bool {
	return request.Metadata["prompt_version"] == "search-intent/v2" && len(request.Messages) == 2 && strings.Contains(request.Messages[1].Content, query)
}

type intentCostRows struct {
	count, micros, promptTokens, completionTokens int64
	provenance                                    bool
}

func (c intentCostRows) matches(wantCount int64) bool {
	return c.count == wantCount && c.micros == wantCount*1000 && c.promptTokens == wantCount*10 && c.completionTokens == wantCount*20 && c.provenance
}

func assertIntentCostRows(t *testing.T, pool *pgxpool.Pool, model string, wantCount int64) {
	t.Helper()
	var rows intentCostRows
	if err := pool.QueryRow(context.Background(), `SELECT count(*), COALESCE(sum(usd_micros), 0),
				COALESCE(sum(prompt_tokens), 0), COALESCE(sum(completion_tokens), 0),
				COALESCE(bool_and(cost_source = 'gateway' AND prompt_version = 'search-intent/v2'
				AND workspace_id IS NULL AND user_id IS NULL), true)
				FROM cost_events WHERE kind = 'search_intent' AND model = $1`, model).
		Scan(&rows.count, &rows.micros, &rows.promptTokens, &rows.completionTokens, &rows.provenance); err != nil {
		t.Fatal(err)
	}
	if !rows.matches(wantCount) {
		t.Fatalf("intent cost rows=%d micros=%d tokens=%d/%d provenance=%v", rows.count, rows.micros, rows.promptTokens, rows.completionTokens, rows.provenance)
	}
}

func analyzedLedgerInterpretation(i catalog.SearchInterpretation) bool {
	return i.Intent["input"] != nil && *i.Intent["input"] == "收支資料" && i.Intent["output"] != nil && *i.Intent["output"] == "報告" && len(i.Intent) == 5 && i.Intent["tools"] == nil && i.Intent["data"] == nil && i.Intent["environment"] == nil && i.PromptVersion == "search-intent/v2"
}
