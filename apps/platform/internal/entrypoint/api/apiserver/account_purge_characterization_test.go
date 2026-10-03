package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type refusingRemoveStore struct{ attempted []string }

func (s *refusingRemoveStore) Remove(_ context.Context, key string) error {
	s.attempted = append(s.attempted, key)
	return errors.New("simulated object-store outage")
}

func dueForPurge(t *testing.T, pool *pgxpool.Pool, c *client) {
	t.Helper()
	if status, _ := deleteJSON(t, c, "/me"); status != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", status)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE users SET deletion_requested_at = '1800-01-01',
		purge_attempted_at = NULL, purge_started_at = NULL WHERE id = $1`, mustUUID(t, c.userID)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE users SET deletion_requested_at = NULL,
			purge_attempted_at = NULL, purge_started_at = NULL WHERE id = $1 AND deleted_at IS NULL`, mustUUID(t, c.userID))
	})
}

func seedWorkspaceDataset(t *testing.T, pool *pgxpool.Pool, c *client, objectKey string) {
	t.Helper()
	ctx := context.Background()
	skillID := seedSkill(t, pool, c.workspaceID, uniqueWorklistLabel("purge-object-skill"))
	var testCaseID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
		VALUES ($1, $2, 'objects', 'objects') RETURNING id`, mustUUID(t, c.workspaceID), mustUUID(t, skillID)).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO datasets
		(workspace_id, test_case_id, file_name, content_type, size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'objects.txt', 'text/plain', 1, 'objects', $3, now() + interval '90 days')`,
		mustUUID(t, c.workspaceID), testCaseID, objectKey); err != nil {
		t.Fatal(err)
	}
}

func accountEmail(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	var email string
	if err := pool.QueryRow(context.Background(), "SELECT email FROM users WHERE id = $1",
		mustUUID(t, userID)).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

const testSchemaLockConnections = 1

func advisoryLocksHeldByTheProduct(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()
	for deadline := time.Now().Add(2 * time.Second); pool.Stat().AcquiredConns() > testSchemaLockConnections && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if leaked := pool.Stat().AcquiredConns() - testSchemaLockConnections; leaked != 0 {
		t.Fatalf("%d connections are still checked out of the pool; their advisory locks cannot be observed", leaked)
	}
	idle := pool.AcquireAllIdle(ctx)
	if len(idle) == 0 {
		t.Fatal("the pool holds no idle connection, so no advisory lock could have been observed")
	}
	pids := make([]int32, 0, len(idle))
	for _, conn := range idle {
		var pid int32
		err := conn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid)
		conn.Release()
		if err != nil {
			t.Fatal(err)
		}
		pids = append(pids, pid)
	}
	return countRows(t, pool,
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND pid = ANY($1)`, pids)
}

func TestAPurgedAccountKeepsNoSessionAndLeavesItsWorkspaceUnlocked(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, uniqueWorklistLabel("purge-sessions-locks"))
	dueForPurge(t, pool, alice)
	if got := countRow(t, pool, "SELECT count(*) FROM sessions WHERE user_id = $1", mustUUID(t, alice.userID)); got == 0 {
		t.Fatal("precondition: the account has no session to remove")
	}

	n, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), &recordingStore{}, 0, 1)
	if err != nil || n != 1 {
		t.Fatalf("purged %d accounts (err %v), want 1", n, err)
	}
	if got := countRow(t, pool, "SELECT count(*) FROM sessions WHERE user_id = $1", mustUUID(t, alice.userID)); got != 0 {
		t.Errorf("%d sessions survived the purge, want 0", got)
	}
	if n := advisoryLocksHeldByTheProduct(t, pool); n != 0 {
		t.Errorf("the workspace object lock is still held after the purge returned: %d advisory locks held", n)
	}
}

func TestAPurgeWhoseObjectCannotBeRemovedStopsBeforeAnyRowChanges(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, uniqueWorklistLabel("purge-remove-fails"))
	key := "datasets/" + uniqueWorklistLabel("purge-remove-fails")
	seedWorkspaceDataset(t, pool, alice, key)
	dueForPurge(t, pool, alice)
	before := accountEmail(t, pool, alice.userID)
	store := &refusingRemoveStore{}

	n, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), store, 0, 1)
	if err == nil || !strings.Contains(err.Error(), "simulated object-store outage") || n != 0 {
		t.Fatalf("purged %d (err %v), want 0 and the store's failure", n, err)
	}
	if len(store.attempted) != 1 || store.attempted[0] != key {
		t.Errorf("removal attempts %v, want exactly [%s]", store.attempted, key)
	}
	if got := accountEmail(t, pool, alice.userID); got != before {
		t.Errorf("the account was de-identified to %q although an object could not be removed", got)
	}
	if got := countRow(t, pool, "SELECT count(*) FROM datasets WHERE object_key = $1", key); got != 1 {
		t.Errorf("%d dataset rows left, want the row kept", got)
	}
	if n := advisoryLocksHeldByTheProduct(t, pool); n != 0 {
		t.Errorf("the workspace object lock is still held after the purge gave up: %d advisory locks held", n)
	}
}

func TestAPurgeThatCannotListAWorkspacesObjectsRemovesNothing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, uniqueWorklistLabel("purge-list-fails"))
	key := "datasets/" + uniqueWorklistLabel("purge-list-fails")
	seedWorkspaceDataset(t, pool, alice, key)
	dueForPurge(t, pool, alice)
	before := accountEmail(t, pool, alice.userID)
	svc := a.auth.Service
	svc.DatasetObjectKeys = func(context.Context, gen.DBTX, pgtype.UUID) ([]string, error) {
		return nil, errors.New("simulated listing failure")
	}
	store := &recordingStore{}

	n, err := svc.PurgeExpiredAccounts(context.Background(), store, 0, 1)
	if err == nil || !strings.Contains(err.Error(), "list testlab object keys: simulated listing failure") || n != 0 {
		t.Fatalf("purged %d (err %v), want 0 and the listing failure named", n, err)
	}
	if len(store.removed) != 0 {
		t.Errorf("removed %v although the object list was incomplete", store.removed)
	}
	if got := accountEmail(t, pool, alice.userID); got != before {
		t.Errorf("the account was de-identified to %q although its objects could not be listed", got)
	}
}

func TestAnAccountWhoseDeletionIsWithdrawnMidSweepIsLeftAlone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, uniqueWorklistLabel("purge-withdrawn"))
	key := "datasets/" + uniqueWorklistLabel("purge-withdrawn")
	seedWorkspaceDataset(t, pool, alice, key)
	dueForPurge(t, pool, alice)
	before := accountEmail(t, pool, alice.userID)
	svc := a.auth.Service
	quiescent := svc.WorkspaceQuiescent
	aliceWorkspace := mustUUID(t, alice.workspaceID)
	readinessChecks := 0
	svc.WorkspaceQuiescent = func(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
		if workspaceID == aliceWorkspace {
			readinessChecks++
		}
		if _, err := pool.Exec(ctx, "UPDATE users SET deletion_requested_at = NULL WHERE id = $1",
			mustUUID(t, alice.userID)); err != nil {
			return false, err
		}
		return quiescent(ctx, db, workspaceID)
	}
	store := &recordingStore{}

	n, err := svc.PurgeExpiredAccounts(context.Background(), store, 0, 1)
	if err != nil || n != 0 {
		t.Fatalf("purged %d (err %v), want 0 and no error", n, err)
	}
	if readinessChecks != 1 {
		t.Fatalf("the sweep checked the withdrawn account's workspace %d times, want 1: the withdrawal was never exercised", readinessChecks)
	}
	if len(store.removed) != 0 {
		t.Errorf("removed %v from an account that is no longer being deleted", store.removed)
	}
	if got := accountEmail(t, pool, alice.userID); got != before {
		t.Errorf("the account was de-identified to %q after its deletion was withdrawn", got)
	}
	if got := countRow(t, pool, "SELECT count(*) FROM users WHERE id = $1 AND purge_started_at IS NULL",
		mustUUID(t, alice.userID)); got != 1 {
		t.Error("the withdrawn account was marked as purging")
	}
}
