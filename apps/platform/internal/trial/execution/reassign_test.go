package run

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func attemptOn(provider string, errClass string) gen.RunAttempt {
	a := gen.RunAttempt{
		ID: pgtype.UUID{Bytes: [16]byte{15: byte(len(provider))}, Valid: true}, Provider: provider,
		CreatedAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC), Valid: true},
	}
	if errClass != "" {
		a.ErrorClass = &errClass
	}
	return a
}

func TestOnlyALostAttemptCountsAsALoss(t *testing.T) {
	attempts := []gen.RunAttempt{
		attemptOn("busy", errClassProvision),
		attemptOn("alpha", errClassProviderLost),
		attemptOn("beta", errClassExecution),
	}
	if got := timesLost(attempts); got != 1 {
		t.Errorf("timesLost = %d, want 1: only the provider_lost attempt counts", got)
	}
	if got := timesLost(nil); got != 0 {
		t.Errorf("timesLost of no attempts = %d, want 0", got)
	}
}

func TestAProviderThatLostThisRunIsKeptOutOfTheNextPlacement(t *testing.T) {
	drained := map[string]gen.DispatchHalt{"gamma": {Provider: "gamma", Source: "operator"}}
	avoid := lostProviders([]gen.RunAttempt{
		attemptOn("alpha", errClassProviderLost),
		attemptOn("beta", errClassExecution),
	}, drained)

	if _, ok := avoid["alpha"]; !ok {
		t.Error("the provider that lost the attempt is still eligible")
	}
	if _, ok := avoid["beta"]; ok {
		t.Error("a provider whose attempt failed in execution was excluded; only a loss excludes one")
	}
	if _, ok := avoid["gamma"]; !ok {
		t.Error("the drained provider was dropped from the exclusions")
	}
	if len(drained) != 1 {
		t.Errorf("the halt map grew to %d entries; it must not be written through", len(drained))
	}
}

func TestOnlyAProviderForgettingTheAttemptIsAnImmediateLoss(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"not found", &providerError{Status: http.StatusNotFound}, true},
		{"server error", &providerError{Status: http.StatusInternalServerError}, false},
		{"too many requests", &providerError{Status: http.StatusTooManyRequests}, false},
		{"network error", errors.New("dial tcp: connection refused"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerForgotAttempt(tc.err); got != tc.want {
				t.Errorf("providerForgotAttempt(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestARunIsReassignableOnlyOnceAndOnlyAfterALoss(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attempts []gen.RunAttempt
		want     bool
	}{
		{"no attempts", nil, false},
		{"last attempt lost", []gen.RunAttempt{attemptOn("alpha", errClassProviderLost)}, true},
		{"last attempt failed in execution", []gen.RunAttempt{attemptOn("alpha", errClassExecution)}, false},
		{"last attempt still live", []gen.RunAttempt{attemptOn("alpha", "")}, false},
		{"two losses", []gen.RunAttempt{
			attemptOn("alpha", errClassProviderLost), attemptOn("beta_", errClassProviderLost),
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reassignableAfter(tc.attempts); got != tc.want {
				t.Errorf("reassignable = %v, want %v", got, tc.want)
			}
		})
	}
}

func usageGateway(t *testing.T, spendPerAttempt float64) *Gateway {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /spend/logs/v2", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":        []any{map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "spend": spendPerAttempt}},
			"total_pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Gateway{AdminBaseURL: srv.URL, SandboxBaseURL: srv.URL, MaxBudgetUSD: 0.50, HTTP: srv.Client()}
}

func TestANewAttemptOnlyGetsWhatIsLeftOfTheRunBudget(t *testing.T) {
	svc := &Service{Gateway: usageGateway(t, 0.10)}
	attempts := []gen.RunAttempt{attemptOn("alpha", errClassProviderLost), attemptOn("beta_", errClassExecution)}

	left, err := svc.budgetLeft(context.Background(), attempts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(left-0.30) > 0.000001 {
		t.Errorf("budget left = %v, want 0.30: 0.50 minus what both earlier attempts spent", left)
	}
}

func TestEveryAttemptsTokensCountAgainstTheSameRunCeiling(t *testing.T) {
	svc := &Service{Gateway: usageGateway(t, 0)}
	attempts := []gen.RunAttempt{attemptOn("alpha", errClassProviderLost), attemptOn("beta_", "")}

	used, err := svc.usageOf(context.Background(), attempts)
	if err != nil {
		t.Fatal(err)
	}
	if used.InputTokens != 20 || used.OutputTokens != 10 {
		t.Errorf("usage = %d in / %d out, want 20 / 10: both attempts counted", used.InputTokens, used.OutputTokens)
	}
}

func TestSpendThatCannotBeReadStopsTheRunFromStartingAnotherAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	svc := &Service{Gateway: &Gateway{AdminBaseURL: srv.URL, MaxBudgetUSD: 0.50, HTTP: srv.Client()}}

	_, err := svc.budgetLeft(context.Background(), []gen.RunAttempt{attemptOn("alpha", errClassProviderLost)})
	if !errors.Is(err, errSpendUnreadable) {
		t.Errorf("budgetLeft error = %v, want it to refuse because the spend is unreadable", err)
	}
}
