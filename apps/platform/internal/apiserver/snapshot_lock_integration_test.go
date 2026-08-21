// The lock that makes a test case snapshot mean what TEST-010 says it means
// (DDD-031, ADR-035 B 組). Shared harness lives in authz_integration_test.go
// (TestMain, migrate, requireDB, newAPI, login, seedSkill); seedTestCase is in
// run_integration_test.go.
//
// Here rather than in internal/testlab because that package has no database
// harness, and adding a fourth package that drops and recreates schema "public"
// buys nothing this file does not already have.

package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
)

// TestCreateSnapshotBlocksOnAConcurrentTestCaseEdit is the invariant
// CreateSnapshot's doc comment promises: two runs that hash the same executed
// the same input. That only holds if nothing can edit the test case between the
// read it hashes and the commit.
//
// Until DDD-031 the lock was taken by internal/run, one statement before the
// call, and this package could not have told you whether it had been taken at
// all. Move it back out - swap the LockDraft inside CreateSnapshot for the
// unlocked GetTestCase - and this test goes red twice over: the freeze does not
// wait, and it hashes a prompt that was overwritten before it committed.
func TestCreateSnapshotBlocksOnAConcurrentTestCaseEdit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "snapshot-lock-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "snapshot-lock-skill")
	testCaseID := seedTestCase(t, pool, owner.workspaceID, skillID)
	ws := mustUUID(t, owner.workspaceID)
	tc := mustUUID(t, testCaseID)
	ctx := context.Background()

	// Two connections, because the point is two transactions running at once and
	// a pool is free to hand the same one to both otherwise.
	editConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer editConn.Release()
	freezeConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer freezeConn.Release()

	edit, err := editConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer edit.Rollback(ctx) //nolint:errcheck // no-op after commit
	freeze, err := freezeConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer freeze.Rollback(ctx) //nolint:errcheck // no-op after commit

	const editedPrompt = "summarise the attached csv, in one paragraph"
	if _, err := edit.Exec(ctx,
		`UPDATE test_cases SET user_prompt = $2 WHERE id = $1`, tc, editedPrompt); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		snapshot gen.TestCaseSnapshot
		err      error
	}
	out := make(chan outcome, 1)
	go func() {
		snapshot, err := testlab.CreateSnapshot(ctx, gen.New(freeze), ws, tc)
		out <- outcome{snapshot, err}
	}()

	if !waitsOnLock(t, pool, freezeConn.Conn().PgConn().PID()) {
		t.Fatal("the freeze did not wait on the edit; CreateSnapshot read the row without locking it")
	}
	if err := edit.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	frozen := <-out
	if frozen.err != nil {
		t.Fatal(frozen.err)
	}
	if frozen.snapshot.UserPrompt != editedPrompt {
		t.Errorf("snapshot froze %q, want the committed %q - the run's record of its own input is wrong",
			frozen.snapshot.UserPrompt, editedPrompt)
	}
	if err := freeze.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestLockDraftIsScopedToItsWorkspace pins the half of the owner-exported
// lock that has nothing to do with concurrency: internal/run hands it a
// workspace id from the session, and a test case outside it has to read as
// missing rather than as a row somebody may lock (WS-006, iron rule 3).
func TestLockDraftIsScopedToItsWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "lock-scope-owner")
	stranger := a.login(t, "lock-scope-stranger")
	skillID := seedSkill(t, pool, owner.workspaceID, "lock-scope-skill")
	testCaseID := seedTestCase(t, pool, owner.workspaceID, skillID)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // read-only

	q := gen.New(tx)
	if _, err := testlab.LockDraft(ctx, q, mustUUID(t, stranger.workspaceID), mustUUID(t, testCaseID)); err != testlab.ErrNotFound {
		t.Errorf("cross-workspace lock err = %v, want ErrNotFound", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE test_cases SET deleted_at = now() WHERE id = $1`, mustUUID(t, testCaseID)); err != nil {
		t.Fatal(err)
	}
	if _, err := testlab.LockDraft(ctx, q, mustUUID(t, owner.workspaceID), mustUUID(t, testCaseID)); err != testlab.ErrNotFound {
		t.Errorf("soft-deleted lock err = %v, want ErrNotFound", err)
	}
}

// waitsOnLock reports whether pid is parked on a lock, which is how these tests
// observe "the other transaction is being made to wait" without a sleep that
// would be a guess in either direction.
func waitsOnLock(t *testing.T, pool *pgxpool.Pool, pid uint32) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := pool.QueryRow(context.Background(),
			`SELECT COALESCE((SELECT wait_event_type = 'Lock' FROM pg_stat_activity WHERE pid = $1), false)`,
			pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
