package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func llmAnswering(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" || r.Header.Get("Authorization") != "Bearer svc-token" {
			http.Error(w, "unexpected request", http.StatusTeapot)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LLM_SERVICE_URL", srv.URL)
	t.Setenv("LLM_SERVICE_TOKEN", "svc-token")
}

func TestTheLLMProbeSaysWhyTheServiceCannotTakeWork(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"ready", http.StatusOK,
			`{"status":"ready","gateway_configured":true,"key_budgeted":true,"missing":[]}`, ""},
		{"a gateway variable is missing", http.StatusOK,
			`{"status":"not_ready","gateway_configured":false,"key_budgeted":false,"missing":["LITELLM_API_KEY"]}`,
			"LITELLM_API_KEY"},
		{"the key has no confirmed budget", http.StatusOK,
			`{"status":"not_ready","gateway_configured":true,"key_budgeted":false,"missing":[]}`,
			"Virtual Key"},
		{"the service token is refused", http.StatusUnauthorized, `{"detail":"invalid service credential"}`,
			"LLM_SERVICE_TOKEN"},
		{"the body is not readiness", http.StatusOK, `not json`, "讀不懂"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			llmAnswering(t, c.status, c.body)
			err := probeLLMService(http.DefaultClient)(context.Background())
			if c.want == "" {
				if err != nil {
					t.Fatalf("got %v, want ready", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error naming %q", err, c.want)
			}
		})
	}
}
