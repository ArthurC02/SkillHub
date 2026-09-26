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

	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestAnonymousSearchTraversesGoPythonAndGateway(t *testing.T) {
	python := creationPythonExecutable(t)
	t.Setenv("LLM_SERVICE_TOKEN", "test-service")
	t.Setenv("INTENT_MODEL", "gpt-5.6-luna")
	pool := requireDB(t)
	const query = "請將收支資料轉成報告"
	for _, tc := range []struct {
		name, finish, status, reason  string
		refuseIntent, refuseEmbedding bool
	}{
		{name: "complete analysis", finish: "stop", status: "analyzed"},
		{name: "truncated analysis keeps vector search", finish: "length", status: "fallback", reason: "invalid_response"},
		{name: "gateway refusal keeps vector search", refuseIntent: true, status: "fallback", reason: "unavailable"},
		{name: "both capabilities unavailable disclose degradation", refuseIntent: true, refuseEmbedding: true, status: "fallback", reason: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := uniqueWorklistLabel("intent-python-cost")
			t.Setenv("INTENT_MODEL", model)
			var analyses, embeddings atomic.Int32
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-service-key" {
					t.Error("unexpected gateway method or credential")
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				var request struct {
					Input    []string          `json:"input"`
					Metadata map[string]string `json:"metadata"`
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-litellm-response-cost", "0.001")
				switch {
				case r.URL.Path == "/v1/embeddings":
					embeddings.Add(1)
					if len(request.Input) != 1 || request.Input[0] != query {
						t.Errorf("embedding input = %v, want original task", request.Input)
					}
					if tc.refuseEmbedding {
						http.Error(w, `{"error":{"message":"unavailable","type":"server_error"}}`, http.StatusServiceUnavailable)
						return
					}
					writeJSON(w, map[string]any{"object": "list", "model": "text-embedding-3-small", "data": []map[string]any{{"object": "embedding", "index": 0, "embedding": unitVector(1486)}}, "usage": map[string]int{"prompt_tokens": 10, "total_tokens": 10}})
				case r.URL.Path == "/v1/chat/completions" && request.Metadata["operation"] == "analyze-intent":
					analyses.Add(1)
					if request.Metadata["prompt_version"] != "search-intent/v2" || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, query) {
						t.Error("intent request lost the task or prompt provenance")
					}
					if tc.refuseIntent {
						http.Error(w, `{"error":{"message":"budget exhausted","type":"budget_exceeded"}}`, http.StatusTooManyRequests)
						return
					}
					content := `{"intent":{"input":"收支資料","output":"報告","tools":null,"data":null,"environment":null},"keywords":["ledger"],"filters":{"script":null,"validation":null,"agent":null,"tier":null,"category":null}}`
					writeJSON(w, map[string]any{"id": "chatcmpl-intent", "object": "chat.completion", "created": 1, "model": "fixture-model", "choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": tc.finish}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
				default:
					http.Error(w, `{"error":{"message":"optional capability unavailable","type":"server_error"}}`, http.StatusServiceUnavailable)
				}
			}))
			t.Cleanup(gateway.Close)
			a := newAPIWithLLM(t, pool, startCreationPython(t, python, gateway.URL))
			owner := a.login(t, uniqueWorklistLabel("intent-python"))
			markCatalog(t, pool, owner.workspaceID)
			t.Cleanup(func() {
				if _, err := pool.Exec(context.Background(), `UPDATE workspaces SET is_catalog = false WHERE id = $1`, mustUUID(t, owner.workspaceID)); err != nil {
					t.Error(err)
				}
			})
			target := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("ledger"))
			seedEmbedding(t, pool, target, 1486)
			res, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape(query))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body struct {
				Interpretation catalog.SearchInterpretation `json:"interpretation"`
				Degraded       bool                         `json:"degraded"`
				Results        []struct {
					ID string `json:"skill_id"`
				} `json:"results"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusOK || body.Interpretation.Status != tc.status || body.Interpretation.FallbackReason != tc.reason || body.Degraded != tc.refuseEmbedding {
				t.Fatalf("status=%d response=%+v", res.StatusCode, body)
			}
			if analyses.Load() != 1 || embeddings.Load() != 1 {
				t.Fatalf("calls: intent=%d embedding=%d; want one each, no retry", analyses.Load(), embeddings.Load())
			}
			var count, micros, promptTokens, completionTokens int64
			var provenance bool
			if err := pool.QueryRow(context.Background(), `SELECT count(*), COALESCE(sum(usd_micros), 0),
				COALESCE(sum(prompt_tokens), 0), COALESCE(sum(completion_tokens), 0),
				COALESCE(bool_and(cost_source = 'gateway' AND prompt_version = 'search-intent/v2'
				AND workspace_id IS NULL AND user_id IS NULL), true)
				FROM cost_events WHERE kind = 'search_intent' AND model = $1`, model).
				Scan(&count, &micros, &promptTokens, &completionTokens, &provenance); err != nil {
				t.Fatal(err)
			}
			wantCount := int64(1)
			if tc.refuseIntent {
				wantCount = 0
			}
			if count != wantCount || micros != wantCount*1000 || promptTokens != wantCount*10 || completionTokens != wantCount*20 || !provenance {
				t.Fatalf("intent cost rows=%d micros=%d tokens=%d/%d provenance=%v", count, micros, promptTokens, completionTokens, provenance)
			}
			found := false
			for _, result := range body.Results {
				found = found || result.ID == target
			}
			if found == tc.refuseEmbedding {
				t.Fatalf("vector-only target present=%v, embedding unavailable=%v", found, tc.refuseEmbedding)
			}
			if tc.status == "analyzed" {
				i := body.Interpretation
				if i.Intent["input"] == nil || *i.Intent["input"] != "收支資料" || i.Intent["output"] == nil || *i.Intent["output"] != "報告" || len(i.Intent) != 5 || i.Intent["tools"] != nil || i.Intent["data"] != nil || i.Intent["environment"] != nil || i.PromptVersion != "search-intent/v2" {
					t.Fatalf("interpretation=%+v", i)
				}
			}
		})
	}
}
