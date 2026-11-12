package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestCreationPythonReturnsMissingRulesToGoWithoutARewrite(t *testing.T) {
	python := creationPythonExecutable(t)
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, schema, ok := readCreationGatewayPrompts(t, w, r)
		if !ok {
			return
		}
		calls.Add(1)
		if schema != "review_diagnosis" {
			t.Errorf("gateway schema = %q, want only the missing-context diagnosis", schema)
		}
		content, err := json.Marshal(map[string]any{"edits": []map[string]string{{
			"criterion": "Use the author's course weights.", "cause": "Course weights were not supplied.",
			"target": "context", "edit": "數學與自然各占總成績的多少比例？",
		}}})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-response-cost", "0.01")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "context-diagnosis", "object": "chat.completion", "created": 1, "model": "fixture-model",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}))
	t.Cleanup(gateway.Close)
	pythonURL := startCreationPython(t, python, gateway.URL)
	provider := &llmclient.Client{BaseURL: pythonURL, Token: "test-service"}
	got, err := provider.CreationStep(context.Background(), llmclient.CreationStepRequest{
		SessionID: "missing-context", Revision: 1, Brief: "Compute a weighted course grade.", BriefConfirmed: true,
		Messages: []llmclient.CreationMessage{
			{Role: "user", Content: "數學 80 分，自然 90 分。"},
			{Role: "tool", Content: `{"evaluation":{"criterion_results":[{"result":"undetermined","reason":"Course weights were not supplied."}]}}`},
		},
		Draft:           &llmclient.GeneratedSkill{Name: "weighted-grades", Description: "Compute the grade using course weights.", Body: "Compute the weighted grade."},
		DraftValidation: &llmclient.CreationDraftValidation{ContentHash: strings.Repeat("c", 64), Report: "{}"},
		GatewayKey:      "test-attempt-key", TimeoutSeconds: 10, MaxOutputTokens: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != "clarification" || got.Message != "數學與自然各占總成績的多少比例？" || got.Draft != nil || got.ToolIntent != nil {
		t.Fatalf("missing-context reply = %+v, want the author's question with no draft or tool", got)
	}
	if calls.Load() != 1 || got.Usage == nil || got.Usage.CostUSD == nil || *got.Usage.CostUSD != 0.01 {
		t.Fatalf("calls=%d usage=%+v, want the single diagnosis charged once", calls.Load(), got.Usage)
	}
}
