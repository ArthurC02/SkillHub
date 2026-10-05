package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func workingAttempt(t *testing.T, pool *pgxpool.Pool, svc *creation.Service, rec *jobRecorder, ws identity.Workspace, activeDeadline string) creation.JobArgs {
	t.Helper()
	ctx := context.Background()
	before := len(rec.calls)
	if _, err := svc.Create(ctx, ws, creationID(t), "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != before+1 {
		t.Fatalf("no job was enqueued for the new session")
	}
	job := rec.calls[before]
	if _, err := pool.Exec(ctx, `UPDATE creation_sessions SET state='working', updated_at=now()-interval '5 minutes',
		snapshot=jsonb_set(snapshot, '{active_deadline}', to_jsonb(now()+$2::interval)) WHERE id=$1`, job.SessionID, activeDeadline); err != nil {
		t.Fatal(err)
	}
	return job
}

func creationState(t *testing.T, pool *pgxpool.Pool, job creation.JobArgs) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(context.Background(), `SELECT state FROM creation_sessions WHERE id=$1`, job.SessionID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestTheRecoverySweepSparesAnAttemptStillWithinItsCallDeadline(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert}
	inFlight := workingAttempt(t, pool, svc, rec, ws, "1 hour")
	overdue := workingAttempt(t, pool, svc, rec, ws, "-1 minute")

	if err := svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := creationState(t, pool, overdue); got != "failed" {
		t.Fatalf("overdue attempt state = %s, want failed", got)
	}
	if got := creationState(t, pool, inFlight); got != "working" {
		t.Errorf("in-flight attempt state = %s, want working", got)
	}
}

func TestAnInterruptedTransientStepIsEndedEvenWithinItsCallDeadline(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert}
	job := workingAttempt(t, pool, svc, rec, ws, "1 hour")

	if err := svc.InterruptedTransient(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if got := creationState(t, pool, job); got != "failed" {
		t.Errorf("state = %s, want failed", got)
	}
}

func TestTheRecoverySweepLeavesAnAttemptStillWrappingUpItsFinishedCall(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert}
	wrappingUp := workingAttempt(t, pool, svc, rec, ws, "-1 minute")
	if _, err := pool.Exec(context.Background(),
		`UPDATE creation_sessions SET updated_at = now() - interval '30 seconds' WHERE id = $1`, wrappingUp.SessionID); err != nil {
		t.Fatal(err)
	}

	if err := svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := creationState(t, pool, wrappingUp); got != "working" {
		t.Errorf("an attempt 30s past a 2s call, still within revoke, search and settle, was swept to %s; its late result would be charged and thrown away", got)
	}
}
