package apiserver_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
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
