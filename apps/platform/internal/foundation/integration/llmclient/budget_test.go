package llmclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
)

func TestOnlyTheNamedPlatformBudgetRefusalComesBackAsItsOwnErrorAndIsCounted(t *testing.T) {
	calls := map[string]func(*Client) error{
		"shared post": func(c *Client) error { _, err := c.Embed(context.Background(), []string{"x"}); return err },
		"creation step": func(c *Client) error {
			_, err := c.CreationStep(context.Background(), CreationStepRequest{GatewayKey: "k"})
			return err
		},
		"agent step": func(c *Client) error {
			_, err := c.AgentStep(context.Background(), AgentStepRequest{GatewayKey: "k"})
			return err
		},
	}
	answers := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"platform budget refusal", http.StatusServiceUnavailable, `{"detail":"model gateway platform budget exhausted"}`, true},
		{"another unavailable cause", http.StatusServiceUnavailable, `{"detail":"LiteLLM gateway not configured"}`, false},
		{"the same words on a bad gateway", http.StatusBadGateway, `{"detail":"model gateway platform budget exhausted"}`, false},
		{"an unavailable body that is not json", http.StatusServiceUnavailable, `model gateway platform budget exhausted`, false},
	}
	for callName, call := range calls {
		for _, answer := range answers {
			t.Run(callName+"/"+answer.name, func(t *testing.T) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(answer.status)
					_, _ = w.Write([]byte(answer.body))
				}))
				defer s.Close()
				before := platformBudgetRefusalsCounted(t)

				err := call(&Client{BaseURL: s.URL})

				if err == nil {
					t.Fatal("a refusal came back as success")
				}
				if got := errors.Is(err, ErrPlatformBudgetExhausted); got != answer.want {
					t.Fatalf("errors.Is(platform budget)=%v, want %v (err=%v)", got, answer.want, err)
				}
				counted := platformBudgetRefusalsCounted(t) - before
				if wantCounted := map[bool]float64{true: 1, false: 0}[answer.want]; counted != wantCounted {
					t.Fatalf("counter moved by %v, want %v", counted, wantCounted)
				}
			})
		}
	}
}

func platformBudgetRefusalsCounted(t *testing.T) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "skillhub_model_platform_budget_exhausted_total" {
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatal("the platform budget refusal counter is not registered")
	return 0
}
