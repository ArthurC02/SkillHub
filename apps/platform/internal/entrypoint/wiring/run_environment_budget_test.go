package wiring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestWithoutAGatewayThereIsNoPlatformBudgetToCheckAgainst(t *testing.T) {
	if PlatformModelBudgetUSD(nil) != nil {
		t.Fatal("a deployment without a gateway got a budget reader that can only fail")
	}
}

func TestThePlatformBudgetReaderAnswersTheGatewaysLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user_info":{"spend":12.5,"max_budget":50.0}}`))
	}))
	defer srv.Close()
	gateway := run.NewGateway(run.GatewayConfig{SandboxBaseURL: srv.URL, AdminBaseURL: srv.URL, AdminKey: "admin"})

	limit, err := PlatformModelBudgetUSD(gateway)(context.Background())

	if err != nil || limit != 50 {
		t.Fatalf("limit = %v, err = %v; want the budget's limit 50, not its spend", limit, err)
	}
}
