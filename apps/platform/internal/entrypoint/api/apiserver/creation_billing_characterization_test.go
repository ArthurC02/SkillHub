package apiserver_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCancelCallsCreditSessionEndedExactlyOnce(t *testing.T) {
	a, _, _ := creationFixture(t)
	service := a.app.CreationSvc
	client := a.login(t, "creation-billing-once")
	view := creationPost(t, client, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_credits": 650}, 200)
	sessionID := mustUUID(t, view.ID)
	var calls atomic.Int32
	service.Billing = creation.BillingHooks{SessionEndedFunc: func(_ context.Context, _ pgx.Tx, got pgtype.UUID) error {
		if got != sessionID {
			t.Errorf("credit summary session = %s, want %s", creation.UUID(got), view.ID)
		}
		calls.Add(1)
		return nil
	}}

	cancelled := creationAct(t, client, view, "cancel")
	if cancelled.State != "cancelled" {
		t.Fatalf("cancel state = %q, want cancelled", cancelled.State)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("CreditSessionEnded calls = %d, want 1", got)
	}
}

func TestCancelCommitsWhenCreditSessionEndedFails(t *testing.T) {
	a, _, _ := creationFixture(t)
	service := a.app.CreationSvc
	client := a.login(t, "creation-billing-error")
	workspace := workspaceOf(t, testPool, client)
	view := creationPost(t, client, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_credits": 650}, 200)
	sessionID := mustUUID(t, view.ID)
	callbackErr := errors.New("cost summary unavailable")
	var calls atomic.Int32
	service.Billing = creation.BillingHooks{SessionEndedFunc: func(_ context.Context, _ pgx.Tx, got pgtype.UUID) error {
		if got != sessionID {
			t.Errorf("credit summary session = %s, want %s", creation.UUID(got), view.ID)
		}
		calls.Add(1)
		return callbackErr
	}}

	cancelled := creationAct(t, client, view, "cancel")
	if cancelled.State != "cancelled" {
		t.Fatalf("cancel state after credit summary error = %q, want cancelled", cancelled.State)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("CreditSessionEnded calls after error = %d, want 1", got)
	}
	stored, err := service.Get(context.Background(), workspace, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "cancelled" {
		t.Fatalf("stored state after credit summary error = %q, want cancelled", stored.State)
	}
}

func TestAStepSettledAfterACancelIsCountedInTheSessionsCostSummary(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-late-settle-summary")
	started := make(chan struct{})
	s.LLM = creationStepFunc(func(ctx context.Context, _ creation.StepRequest) (*creation.StepResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	var mu sync.Mutex
	var order []string
	note := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, what)
	}
	s.Billing = creation.BillingHooks{
		SettleFunc:       func(context.Context, pgx.Tx, creation.StepSettlement) error { note("settle"); return nil },
		SessionEndedFunc: func(context.Context, pgx.Tx, pgtype.UUID) error { note("summary"); return nil },
	}
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_credits": 650}, 200)
	job := creationJob(t, v.ID)
	done := make(chan error, 1)
	go func() { done <- s.Step(context.Background(), job, nil) }()
	<-started
	working, err := s.Get(context.Background(), identity.Workspace{ID: job.WorkspaceID}, job.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	creationAct(t, c, working, "cancel")
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 || order[len(order)-2] != "settle" || order[len(order)-1] != "summary" {
		t.Errorf("billing calls = %v, want the summary rewritten after the late step settled", order)
	}
}

func TestAStepSettledAfterItsInterruptedSessionWasCancelledIsCountedInTheCostSummary(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-abandoned-settle-summary")
	started, release := make(chan struct{}), make(chan struct{})
	s.LLM = creationStepFunc(func(context.Context, creation.StepRequest) (*creation.StepResult, error) {
		close(started)
		<-release
		return nil, errors.New("the reply came back after the session moved on")
	})
	var mu sync.Mutex
	var order []string
	note := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, what)
	}
	s.Billing = creation.BillingHooks{
		SettleFunc:       func(context.Context, pgx.Tx, creation.StepSettlement) error { note("settle"); return nil },
		SessionEndedFunc: func(context.Context, pgx.Tx, pgtype.UUID) error { note("summary"); return nil },
	}
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "開始創作", "budget_credits": 650}, 200)
	job := creationJob(t, v.ID)
	done := make(chan error, 1)
	go func() { done <- s.Step(context.Background(), job, nil) }()
	<-started
	if err := s.InterruptedTransient(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	interrupted, err := s.Get(context.Background(), identity.Workspace{ID: job.WorkspaceID}, job.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	creationAct(t, c, interrupted, "cancel")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 || order[len(order)-2] != "settle" || order[len(order)-1] != "summary" {
		t.Errorf("billing calls = %v, want the summary rewritten after the abandoned step settled", order)
	}
}
