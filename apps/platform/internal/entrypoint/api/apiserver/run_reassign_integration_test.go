package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

type lossScene struct {
	ctx         context.Context
	pool        *pgxpool.Pool
	f           fixture
	svc         run.Service
	alpha, beta *providertest.Fake
	ws, runID   pgtype.UUID
}

func newLossScene(t *testing.T, name string) lossScene {
	t.Helper()
	return lossSceneWith(t, name, nil, true)
}

func newLossSceneWithGateway(t *testing.T, name string, gateway *run.Gateway) lossScene {
	t.Helper()
	return lossSceneWith(t, name, gateway, true)
}

func newSoleProviderScene(t *testing.T, name string) lossScene {
	t.Helper()
	return lossSceneWith(t, name, nil, false)
}

func lossSceneWith(t *testing.T, name string, gateway *run.Gateway, spare bool) lossScene {
	t.Helper()
	pool := requireDB(t)
	a := newAPI(t, pool)
	s := lossScene{ctx: context.Background(), pool: pool, f: newFixture(t, a, pool, name)}
	clearRunBacklog(t, pool)

	s.alpha = providertest.New("alpha_sandbox", "test-token")
	t.Cleanup(s.alpha.Close)
	s.alpha.Plan = providertest.Plan{StuckRunning: true}
	s.alpha.SetFreeSlots(4)
	registry := run.NewRegistry(s.alpha.Provider())
	if spare {
		s.beta = providertest.New("beta_sandbox", "test-token")
		t.Cleanup(s.beta.Close)
		s.beta.SetFreeSlots(2)
		registry = run.NewRegistry(s.alpha.Provider(), s.beta.Provider())
	}

	s.svc = *a.runs
	s.svc.Providers = registry
	s.svc.Providers.TTL = time.Millisecond
	s.svc.Store = a.packages
	if gateway != nil {
		s.svc.Gateway = gateway
	}

	created := s.f.start(t)
	s.ws, s.runID = mustUUID(t, s.f.workspaceID), mustUUID(t, created.RunID)
	s.drive(t, "the first dispatch")
	if s.alpha.Dispatches() != 1 {
		t.Fatalf("precondition: alpha took %d dispatches, want 1", s.alpha.Dispatches())
	}
	return s
}

func (s lossScene) drive(t *testing.T, what string) {
	t.Helper()
	err := s.svc.Drive(s.ctx, s.ws, s.runID)
	if err != nil && !errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("driving %s: %v", what, err)
	}
}

func (s lossScene) attempts(t *testing.T) []gen.RunAttempt {
	t.Helper()
	attempts, err := s.svc.Attempts(s.ctx, s.ws, s.runID)
	if err != nil {
		t.Fatal(err)
	}
	return attempts
}

func (s lossScene) silentFor(t *testing.T, d time.Duration) {
	t.Helper()
	if _, err := s.pool.Exec(s.ctx, `UPDATE run_attempts SET provider_unreachable_since = now() - $2::interval
		WHERE run_id = $1 AND finished_at IS NULL`, s.runID, fmt.Sprintf("%d milliseconds", d.Milliseconds())); err != nil {
		t.Fatal(err)
	}
}

func (s lossScene) silentSince(t *testing.T) pgtype.Timestamptz {
	t.Helper()
	var since pgtype.Timestamptz
	if err := s.pool.QueryRow(s.ctx, `SELECT provider_unreachable_since FROM run_attempts
		WHERE run_id = $1`, s.runID).Scan(&since); err != nil {
		t.Fatal(err)
	}
	return since
}

func errorClass(a gen.RunAttempt) string {
	if a.ErrorClass == nil {
		return ""
	}
	return *a.ErrorClass
}

func TestAProviderThatForgetsAnAttemptHandsTheRunToAnother(t *testing.T) {
	s := newLossScene(t, "alice-forgotten-attempt")
	s.alpha.SetPollStatus(http.StatusNotFound)

	s.drive(t, "the run whose provider forgot it")

	attempts := s.attempts(t)
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2 (the lost one and the reassignment)", len(attempts))
	}
	if attempts[0].Provider != "alpha_sandbox" || errorClass(attempts[0]) != "provider_lost" {
		t.Errorf("first attempt = %s/%s, want alpha_sandbox/provider_lost", attempts[0].Provider, errorClass(attempts[0]))
	}
	if attempts[1].Provider != "beta_sandbox" || attempts[1].ProviderRunID == nil {
		t.Errorf("second attempt = %s with handle %v, want beta_sandbox with one",
			attempts[1].Provider, attempts[1].ProviderRunID)
	}
	_, view := s.f.getRun(t, uuidText(s.runID))
	if view.Status != string(gen.RunStatusSucceeded) {
		t.Errorf("run = %q (%s), want succeeded on the second provider", view.Status, view.StatusReason)
	}
	if view.Provider != "beta_sandbox" {
		t.Errorf("run.provider = %q, want the provider that accepted the last attempt", view.Provider)
	}
}

func TestAProviderIsOnlyLostAfterItHasBeenSilentLongEnough(t *testing.T) {
	s := newLossScene(t, "alice-silent-provider")
	s.alpha.SetPollStatus(http.StatusInternalServerError)

	s.drive(t, "the first failed poll")
	if got := len(s.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d after one failed poll, want 1: one bad poll is not a loss", got)
	}

	s.silentFor(t, run.ProviderLostAfter-time.Second)
	s.drive(t, "a provider silent for just under the limit")
	if got := len(s.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d one second short of the limit, want 1", got)
	}

	s.silentFor(t, run.ProviderLostAfter)
	s.drive(t, "a provider silent for the whole limit")
	attempts := s.attempts(t)
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d at the limit, want 2: the provider is lost", len(attempts))
	}
	if errorClass(attempts[0]) != "provider_lost" || attempts[1].Provider != "beta_sandbox" {
		t.Errorf("attempts = %s/%s then %s, want provider_lost then beta_sandbox",
			attempts[0].Provider, errorClass(attempts[0]), attempts[1].Provider)
	}
}

func TestAProviderThatAnswersAgainKeepsItsAttempt(t *testing.T) {
	s := newLossScene(t, "alice-provider-returns")
	s.alpha.SetPollStatus(http.StatusInternalServerError)
	s.drive(t, "the first failed poll")
	if !s.silentSince(t).Valid {
		t.Fatal("precondition: a failed poll left no record of when the provider went quiet")
	}

	s.alpha.SetPollStatus(0)
	s.drive(t, "the poll that answered again")

	if got := len(s.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
	if s.silentSince(t).Valid {
		t.Error("the silence was still on record after the provider answered; the next hiccup would count from the first one")
	}
}

func TestASecondLossEndsTheRunAsAProviderError(t *testing.T) {
	s := newLossScene(t, "alice-lost-twice")
	s.alpha.SetPollStatus(http.StatusNotFound)
	s.beta.Plan = providertest.Plan{StuckRunning: true}
	s.beta.SetPollStatus(http.StatusNotFound)

	s.drive(t, "the run losing both providers")

	attempts := s.attempts(t)
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2: a run is reassigned once", len(attempts))
	}
	_, view := s.f.getRun(t, uuidText(s.runID))
	if view.Status != string(gen.RunStatusFailed) || view.FailureClass.Value != "provider_error" {
		t.Fatalf("run = %q/%q, want failed/provider_error", view.Status, view.FailureClass.Value)
	}
	if !strings.Contains(view.StatusReason, "改派過一次") {
		t.Errorf("status_reason = %q, want it to say the run had already been reassigned", view.StatusReason)
	}
}

func TestTheProviderThatLostTheRunIsNotGivenItAgain(t *testing.T) {
	s := newSoleProviderScene(t, "alice-single-provider")
	s.alpha.SetPollStatus(http.StatusNotFound)

	s.drive(t, "the run whose only provider lost it")

	if got := len(s.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d, want 1: the lost provider must not take the run again", got)
	}
	_, view := s.f.getRun(t, uuidText(s.runID))
	if view.Status != string(gen.RunStatusFailed) {
		t.Fatalf("run = %q (%s), want failed", view.Status, view.StatusReason)
	}
	if !strings.Contains(view.StatusReason, "alpha_sandbox") {
		t.Errorf("status_reason = %q, want it to name the provider that is out of the running", view.StatusReason)
	}
}

func TestARunReassignedAfterARestartIsDispatchedAgainInsteadOfTerminated(t *testing.T) {
	s := newLossScene(t, "alice-restart-after-loss")
	if _, err := s.pool.Exec(s.ctx, `UPDATE run_attempts SET error_class = 'provider_lost',
		error_message = 'the node went away', finished_at = now() WHERE run_id = $1`, s.runID); err != nil {
		t.Fatal(err)
	}

	s.drive(t, "the run resumed after a restart")

	attempts := s.attempts(t)
	if len(attempts) != 2 || attempts[1].Provider != "beta_sandbox" {
		t.Fatalf("attempts = %d ending on %v, want a second one on beta_sandbox",
			len(attempts), attempts[len(attempts)-1].Provider)
	}
}

type keyMint struct {
	mu      sync.Mutex
	budgets []float64
}

func (m *keyMint) gateway(t *testing.T, spendPerAttempt float64) *run.Gateway {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /key/generate", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxBudget float64 `json:"max_budget"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.budgets = append(m.budgets, body.MaxBudget)
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"key": "sk-test"})
	})
	mux.HandleFunc("GET /spend/logs/v2", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":        []any{map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "spend": spendPerAttempt}},
			"total_pages": 1,
		})
	})
	mux.HandleFunc("POST /key/delete", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &run.Gateway{AdminBaseURL: srv.URL, SandboxBaseURL: srv.URL, MaxBudgetUSD: 0.50, HTTP: srv.Client()}
}

func TestAReassignedAttemptOnlyGetsWhatIsLeftOfTheRunBudget(t *testing.T) {
	mint := &keyMint{}
	s := newLossSceneWithGateway(t, "alice-shared-budget", mint.gateway(t, 0.12))
	s.alpha.SetPollStatus(http.StatusNotFound)

	s.drive(t, "the run losing its provider")

	mint.mu.Lock()
	defer mint.mu.Unlock()
	if len(mint.budgets) != 2 {
		t.Fatalf("keys minted = %v, want one per attempt", mint.budgets)
	}
	if mint.budgets[0] != 0.50 {
		t.Errorf("the first attempt got %v, want the whole run budget 0.50", mint.budgets[0])
	}
	if diff := mint.budgets[1] - 0.38; diff > 0.000001 || diff < -0.000001 {
		t.Errorf("the second attempt got %v, want 0.38: what is left after the first spent 0.12", mint.budgets[1])
	}
}

func TestARunWhoseSpendCannotBeReadIsNotReassigned(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/key/generate" {
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "sk-test"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)
	s := newLossSceneWithGateway(t, "alice-unreadable-spend",
		&run.Gateway{AdminBaseURL: broken.URL, SandboxBaseURL: broken.URL, MaxBudgetUSD: 0.50, HTTP: broken.Client()})
	s.alpha.SetPollStatus(http.StatusNotFound)

	s.drive(t, "the run whose spend cannot be read")

	if got := len(s.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d, want 1: without a readable spend the budget cannot be honoured", got)
	}
	_, view := s.f.getRun(t, uuidText(s.runID))
	if view.Status != string(gen.RunStatusFailed) {
		t.Errorf("run = %q (%s), want failed", view.Status, view.StatusReason)
	}
}
