package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func removeCreationWorkspace(t *testing.T, pool *pgxpool.Pool, ws identity.Workspace) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		var owner pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT owner_user_id FROM workspaces WHERE id = $1`, ws.ID).Scan(&owner); err != nil {
			t.Errorf("cleanup: workspace owner: %v", err)
			return
		}
		for _, step := range []struct {
			stmt string
			arg  pgtype.UUID
		}{
			{`DELETE FROM creation_receipts WHERE workspace_id = $1`, ws.ID},
			{`DELETE FROM creation_session_events WHERE workspace_id = $1`, ws.ID},
			{`DELETE FROM creation_sessions WHERE workspace_id = $1`, ws.ID},
			{`DELETE FROM workspaces WHERE id = $1`, ws.ID},
			{`DELETE FROM users WHERE id = $1`, owner},
		} {
			if _, err := pool.Exec(ctx, step.stmt, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.stmt, err)
			}
		}
	})
}

func TestARefusedAttemptLeavesNoActiveReceiptBehind(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert, LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{deadline}', to_jsonb($2::text)) WHERE id=$1`, id, past); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), rec.calls[0], nil); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	var active *string
	if err := pool.QueryRow(context.Background(), `SELECT snapshot->>'active_receipt' FROM creation_sessions WHERE id=$1`, id).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Errorf("active receipt after the refusal = %s, want none", *active)
	}
}
