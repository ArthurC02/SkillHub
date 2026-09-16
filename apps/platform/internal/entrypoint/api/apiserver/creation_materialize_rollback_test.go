package apiserver_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type materializationWorkspaceRows struct {
	skills, versions, sources, testCases, outbox int
}

func materializationRows(t *testing.T, pool *pgxpool.Pool, workspaceID pgtype.UUID) materializationWorkspaceRows {
	t.Helper()
	var rows materializationWorkspaceRows
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM skills WHERE workspace_id=$1),
			(SELECT count(*) FROM skill_versions WHERE workspace_id=$1),
			(SELECT count(*) FROM skill_sources WHERE workspace_id=$1),
			(SELECT count(*) FROM test_cases WHERE workspace_id=$1),
			(SELECT count(*) FROM outbox_events WHERE workspace_id=$1)`, workspaceID,
	).Scan(&rows.skills, &rows.versions, &rows.sources, &rows.testCases, &rows.outbox); err != nil {
		t.Fatal(err)
	}
	return rows
}

func creationHistoryRows(t *testing.T, pool *pgxpool.Pool, workspaceID, id pgtype.UUID) (int, int) {
	t.Helper()
	var events, receipts int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM creation_session_events WHERE session_id=$1 AND workspace_id=$2),
			(SELECT count(*) FROM creation_receipts WHERE session_id=$1 AND workspace_id=$2)`, id, workspaceID,
	).Scan(&events, &receipts); err != nil {
		t.Fatal(err)
	}
	return events, receipts
}

func TestMaterializeRollsBackCandidateAndSessionWhenAcceptanceTestCaseFails(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-materialize-rollback")
	ws := workspaceOf(t, testPool, alice)
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)
	v = creationStep(t, s, v)
	if v.Snapshot.PendingAction != "confirm_brief" || len(v.Snapshot.AcceptanceCriteria) == 0 {
		t.Fatalf("brief proposal = %+v, want pending confirmation with acceptance criteria", v.Snapshot)
	}
	v = creationAct(t, alice, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.State != "draft_ready" || v.Snapshot.Draft == nil {
		t.Fatalf("draft after confirmation = %+v, want draft_ready", v)
	}

	id := mustUUID(t, v.ID)
	beforeState, beforeRevision, beforeSnapshot := creationSessionRow(t, testPool, ws.ID, id)
	beforeEvents, beforeReceipts := creationHistoryRows(t, testPool, ws.ID, id)
	beforeWorkspaceRows := materializationRows(t, testPool, ws.ID)
	sentinel := errors.New("acceptance test case rejected")
	callbackWorkspaces := make(chan pgtype.UUID, 1)
	a.app.CreationSvc.CreateAcceptanceTestCase = func(_ context.Context, _ pgx.Tx, callbackWorkspace identity.Workspace, _ string, _ string, _ string, _ []string) (string, error) {
		callbackWorkspaces <- callbackWorkspace.ID
		return "", sentinel
	}

	status, _ := creationPostStatus(t, alice, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "materialize", "content_hash": v.Snapshot.Draft.ContentHash,
	})
	if status != 503 {
		t.Errorf("materialize status = %d, want 503", status)
	}
	select {
	case callbackWorkspaceID := <-callbackWorkspaces:
		if callbackWorkspaceID != ws.ID {
			t.Errorf("acceptance test case workspace = %s, want %s", creation.UUID(callbackWorkspaceID), creation.UUID(ws.ID))
		}
	default:
		t.Fatal("acceptance test case callback was not called")
	}

	afterState, afterRevision, afterSnapshot := creationSessionRow(t, testPool, ws.ID, id)
	afterEvents, afterReceipts := creationHistoryRows(t, testPool, ws.ID, id)
	afterWorkspaceRows := materializationRows(t, testPool, ws.ID)
	if afterState != beforeState || afterRevision != beforeRevision || !bytes.Equal(afterSnapshot, beforeSnapshot) {
		t.Fatalf("session changed from state %q revision %d snapshot %s to state %q revision %d snapshot %s", beforeState, beforeRevision, beforeSnapshot, afterState, afterRevision, afterSnapshot)
	}
	if afterEvents != beforeEvents || afterReceipts != beforeReceipts {
		t.Fatalf("session history changed from events=%d receipts=%d to events=%d receipts=%d", beforeEvents, beforeReceipts, afterEvents, afterReceipts)
	}
	if afterWorkspaceRows != beforeWorkspaceRows {
		t.Fatalf("materialization rows changed from %+v to %+v", beforeWorkspaceRows, afterWorkspaceRows)
	}
	after, err := a.app.CreationSvc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Snapshot.Candidate != nil {
		t.Fatalf("candidate after failed materialize = %+v, want nil", after.Snapshot.Candidate)
	}
}
