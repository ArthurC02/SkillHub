package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderReturnsTheCheckedSourceAndRecordsTheSearchQuery(t *testing.T) {
	p := &provider{}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"local source proof"}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"url":"https://example.test/checked-source"`) || len(p.calls) != 1 || p.calls[0]["query"] != "local source proof" {
		t.Fatalf("search fixture lost its source or observations: %d %s %+v", w.Code, w.Body, p.calls)
	}
}

func TestProviderActuallyCallsTheInterceptedSearchToolAndReflectsItsDenial(t *testing.T) {
	for _, tc := range []struct{ name, path, body, kind string }{
		{"chat function call", "/v1/chat/completions", `{"tools":[{"type":"function","function":{"name":"litellm_web_search"}}]}`, "tool_calls"},
		{"responses function call", "/v1/responses", `{"tools":[{"type":"function","name":"litellm_web_search"}]}`, "function_call"},
		{"denied search result", "/v1/responses", `{"tools":[{"type":"function","name":"litellm_web_search"}],"error":"not allowed to access search tool"}`, "not allowed to access search tool"},
		{"ordinary function", "/v1/chat/completions", `{"tools":[{"type":"function","function":{"name":"format_result"}}]}`, "local model proof"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &provider{}
			w := httptest.NewRecorder()
			p.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.kind) || len(p.models) != 1 {
				t.Fatalf("model fixture did not exercise the requested behavior: %d %s %+v", w.Code, w.Body, p.models)
			}
			stats := httptest.NewRecorder()
			p.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/stats", nil))
			var got struct {
				Models []any `json:"model_calls"`
			}
			if err := json.Unmarshal(stats.Body.Bytes(), &got); err != nil || len(got.Models) != 1 {
				t.Fatalf("model observations were not returned: %s %v", stats.Body, err)
			}
		})
	}
}

func TestProviderRejectsInvalidJSONWithoutRecordingASuccess(t *testing.T) {
	p := &provider{}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/search", strings.NewReader("{")))
	if w.Code != http.StatusBadRequest || len(p.calls) != 0 || len(p.models) != 0 {
		t.Fatalf("invalid input became a successful observation: HTTP %d", w.Code)
	}
}
