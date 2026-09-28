package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestACreationKeyIsBoundToTheCreationModelBudgetAndLifetime(t *testing.T) {
	t.Setenv("CREATION_MODEL", "creation-role-under-test")
	var minted struct {
		Duration  string            `json:"duration"`
		MaxBudget float64           `json:"max_budget"`
		Models    []string          `json:"models"`
		Metadata  map[string]string `json:"metadata"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&minted); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"key":"minted-creation-key"}`))
	}))
	t.Cleanup(srv.Close)
	s := &creation.Service{}
	wireCreationGateway(s, run.NewGateway(run.GatewayConfig{
		AdminBaseURL: srv.URL, AdminKey: "test", SandboxBaseURL: srv.URL, HTTP: srv.Client(),
	}))

	key, err := s.IssueKey(context.Background(), "session-1", "receipt-1", .2, 45*time.Second)
	if err != nil || key != "minted-creation-key" {
		t.Fatalf("IssueKey = %q, %v; want the minted key", key, err)
	}
	if !slices.Equal(minted.Models, []string{"creation-role-under-test"}) {
		t.Errorf("models = %v, want only the creation model", minted.Models)
	}
	if minted.MaxBudget != .2 || minted.Duration != "45s" {
		t.Errorf("budget=%v duration=%q, want 0.2 and 45s", minted.MaxBudget, minted.Duration)
	}
	if minted.Metadata["creation_session_id"] != "session-1" || minted.Metadata["creation_attempt_id"] != "receipt-1" {
		t.Errorf("metadata = %v, want the session and the receipt", minted.Metadata)
	}
}
