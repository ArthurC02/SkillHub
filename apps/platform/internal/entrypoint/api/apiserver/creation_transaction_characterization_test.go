package apiserver_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func rejectCreationRiverJob(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION skillhub_test_reject_creation_job() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected creation River insert failure'; END $$;
		CREATE TRIGGER skillhub_test_reject_creation_job
		BEFORE INSERT ON river_job
		FOR EACH ROW WHEN (NEW.kind = 'creation_step')
		EXECUTE FUNCTION skillhub_test_reject_creation_job()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS skillhub_test_reject_creation_job ON river_job;
			DROP FUNCTION IF EXISTS skillhub_test_reject_creation_job()`); err != nil {
			t.Error(err)
		}
	})
}

func assertCreationTransactionRows(t *testing.T, pool *pgxpool.Pool, workspaceID, id pgtype.UUID, wantSessions, wantEvents, wantReceipts, wantJobs int) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		name  string
		query string
		want  int
	}{
		{"sessions", "SELECT count(*) FROM creation_sessions WHERE id=$1 AND workspace_id=$2", wantSessions},
		{"events", "SELECT count(*) FROM creation_session_events WHERE session_id=$1 AND workspace_id=$2", wantEvents},
		{"receipts", "SELECT count(*) FROM creation_receipts WHERE session_id=$1 AND workspace_id=$2", wantReceipts},
		{"River jobs", "SELECT count(*) FROM river_job WHERE kind='creation_step' AND args->>'session_id'=$1 AND args->>'workspace_id'=$2", wantJobs},
	}
	for _, check := range checks {
		var got int
		if err := pool.QueryRow(ctx, check.query, uuidString(id), uuidString(workspaceID)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("%s for session %s = %d, want %d", check.name, uuidString(id), got, check.want)
		}
	}
}

func creationSessionRow(t *testing.T, pool *pgxpool.Pool, workspaceID, id pgtype.UUID) (string, int64, []byte) {
	t.Helper()
	var state string
	var revision int64
	var snapshot []byte
	if err := pool.QueryRow(context.Background(), "SELECT state, revision, snapshot FROM creation_sessions WHERE id=$1 AND workspace_id=$2", id, workspaceID).Scan(&state, &revision, &snapshot); err != nil {
		t.Fatal(err)
	}
	return state, revision, snapshot
}

func TestCreateRollsBackSessionEventReceiptAndRiverJobWhenRiverInsertFails(t *testing.T) {
	pool := requireDB(t)
	a, _, _ := creationFixture(t)
	ws := workspaceOf(t, pool, a.login(t, "creation-transaction-create"))
	id := creationID(t)
	rejectCreationRiverJob(t, pool)

	_, err := a.app.CreationSvc.Create(context.Background(), ws, id, "queue this creation", .5)
	if err == nil || !strings.Contains(err.Error(), "injected creation River insert failure") {
		t.Errorf("Create error = %v, want injected River insert failure", err)
	}
	assertCreationTransactionRows(t, pool, ws.ID, id, 0, 0, 0, 0)
}

func TestActRollsBackAdvanceEventReceiptAndRiverJobWhenRiverInsertFails(t *testing.T) {
	pool := requireDB(t)
	a, _, _ := creationFixture(t)
	ws := workspaceOf(t, pool, a.login(t, "creation-transaction-act"))
	id := creationID(t)
	before, err := a.app.CreationSvc.Create(context.Background(), ws, id, "", .5)
	if err != nil {
		t.Fatal(err)
	}
	if before.State != "waiting_input" {
		t.Fatalf("fixture state = %q, want waiting_input", before.State)
	}
	beforeState, beforeRevision, beforeSnapshot := creationSessionRow(t, pool, ws.ID, id)
	rejectCreationRiverJob(t, pool)

	_, _, err = a.app.CreationSvc.Act(context.Background(), ws, id, creation.Command{
		ID: creationID(t), ExpectedRevision: before.Revision, Kind: "message", Message: "queue this next step",
	})
	if err == nil || !strings.Contains(err.Error(), "injected creation River insert failure") {
		t.Errorf("Act error = %v, want injected River insert failure", err)
	}
	assertCreationTransactionRows(t, pool, ws.ID, id, 1, 1, 0, 0)
	afterState, afterRevision, afterSnapshot := creationSessionRow(t, pool, ws.ID, id)
	if afterState != beforeState || afterRevision != beforeRevision || !bytes.Equal(afterSnapshot, beforeSnapshot) {
		t.Fatalf("session after failed Act changed from state %q revision %d snapshot %s to state %q revision %d snapshot %s", beforeState, beforeRevision, beforeSnapshot, afterState, afterRevision, afterSnapshot)
	}
}
