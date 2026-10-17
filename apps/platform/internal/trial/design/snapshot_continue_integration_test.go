package testlab

import (
	"context"
	"testing"
)

func TestContinuingASnapshotFreezesANewOneForTheNewPromptAndLeavesTheFirst(t *testing.T) {
	pool := requireTestLabDB(t)
	ctx := context.Background()
	ws, testCase, _, _ := seedTwoCases(t, pool)
	svc := &Service{Pool: pool}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	first, err := svc.CreateSnapshot(ctx, tx, ws.ID, testCase)
	if err != nil {
		t.Fatal(err)
	}
	same, err := svc.ContinueSnapshot(ctx, tx, ws.ID, first.ID, first.UserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	next, err := svc.ContinueSnapshot(ctx, tx, ws.ID, first.ID, "now do it in French")
	if err != nil {
		t.Fatal(err)
	}

	if same.ContentHash != first.ContentHash {
		t.Errorf("continuing with the same prompt hashed %q, want the first snapshot's %q", same.ContentHash, first.ContentHash)
	}
	if next.ID == first.ID || next.UserPrompt != "now do it in French" || next.TestCaseID != first.TestCaseID {
		t.Errorf("continuation = %+v, want a new snapshot of the same Test Case with the new prompt", next)
	}
	if next.ContentHash == first.ContentHash {
		t.Errorf("a new prompt kept the first snapshot's content hash %q", first.ContentHash)
	}
	var stored string
	if err := tx.QueryRow(ctx, `SELECT user_prompt FROM test_case_snapshots WHERE id = $1`, first.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != first.UserPrompt {
		t.Errorf("the first snapshot's prompt became %q", stored)
	}
}
