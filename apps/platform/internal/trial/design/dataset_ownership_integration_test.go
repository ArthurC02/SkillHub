package testlab

// DELETE /test-cases/{id}/datasets/{datasetId} used to read only {datasetId}:
// the row was loaded by (dataset id, workspace id) and the {id} in the URL was
// never compared to it, so deleting one test case's file through another test
// case's URL succeeded and answered 200. Same workspace either way — the session
// still supplies the scope — but the path asserted a parent-child relationship
// the code did not check, while the sibling DeleteCriterion in the same file did.
//
// Needs PostgreSQL: what is under test is a comparison against a column, and a
// mock would only restate the Go line above it. Point
// SKILLHUB_TEST_DATABASE_URL at a throwaway database and it runs; leave it unset
// and it skips.
//
// WARNING: TestMain drops and recreates schema "public" in that database.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
)

const testLabDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var testLabPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(testLabDBURLEnv)
	if dsn == "" {
		// 02:PORT-004. Without this, an unset or misspelled URL is indistinguishable
		// from a passing run: every database test removes itself and go test still
		// prints ok. CI sets SKILLHUB_REQUIRE_DB=1 so the service failing to come up
		// is a red build rather than a quiet one.
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", testLabDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run()) // the database tests skip; see requireTestLabDB
	}
	if err := validateDestructiveTestLabDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migrateTestLabSchema(ctx, pool); err != nil {
		panic(err)
	}
	testLabPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

// validateDestructiveTestLabDatabaseURL refuses to point the schema drop below
// at anything that is not an obviously disposable local database.
func validateDestructiveTestLabDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", testLabDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", testLabDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", testLabDBURLEnv)
	}
	return nil
}

func TestDestructiveTestLabDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
	} {
		if err := validateDestructiveTestLabDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructiveTestLabDatabaseURL("postgres://u:p@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func migrateTestLabSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		// No arguments means the simple protocol, so a file with several
		// statements applies as one batch.
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// lockTestSchema serialises the packages that reset this database. apiserver,
// eval, registry and this package each drop and recreate schema "public" in
// SKILLHUB_TEST_DATABASE_URL, and `go test ./...` runs packages concurrently.
// Session scoped, so a crashed run releases it with its connection.
func lockTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	return func() {
		_, _ = conn.Exec(ctx,
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	}
}

func requireTestLabDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testLabPool == nil {
		t.Skipf("%s not set; skipping dataset ownership test", testLabDBURLEnv)
	}
	return testLabPool
}

// removedStore records the keys DeleteDataset asks storage to drop, which is the
// second half of the bug: the wrong file's bytes went with the wrong row.
type removedStore struct{ removed []string }

func (s *removedStore) Put(context.Context, string, []byte) error { return nil }

func (s *removedStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}

type retryRemovalStore struct {
	removed int
	err     error
	key     string
}

func (s *retryRemovalStore) Put(_ context.Context, key string, _ []byte) error {
	s.key = key
	return nil
}
func (*retryRemovalStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}
func (*retryRemovalStore) Exists(context.Context, string) (bool, error) { return true, nil }
func (s *retryRemovalStore) Remove(context.Context, string) error {
	s.removed++
	return s.err
}

func (s *removedStore) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	return nil
}

type blockingPutStore struct {
	started chan struct{}
	release chan struct{}
	key     string
}

func (s *blockingPutStore) Put(ctx context.Context, key string, _ []byte) error {
	s.key = key
	close(s.started)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (*blockingPutStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}

func (*blockingPutStore) Remove(context.Context, string) error { return nil }

type cancelingPutStore struct {
	cancel       context.CancelFunc
	removeCalled bool
	removeCtxErr error
}

func (s *cancelingPutStore) Put(context.Context, string, []byte) error {
	s.cancel()
	return nil
}

func (*cancelingPutStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}

func (s *cancelingPutStore) Remove(ctx context.Context, _ string) error {
	s.removeCalled = true
	s.removeCtxErr = ctx.Err()
	return nil
}

// seedTwoCases writes one workspace holding two test cases, with one file on the
// second. Raw SQL rather than the service: CreateTestCase needs Registry's skill
// reader and UploadDataset needs magic-byte-valid content, and neither is what
// this test is about.
func seedTwoCases(t *testing.T, pool *pgxpool.Pool) (ws identity.Workspace, caseA, caseB, datasetB pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	tag := fmt.Sprintf("%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())

	err := pool.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (email, display_name) VALUES ($1 || '@example.test', $1)
			RETURNING id
		), w AS (
			INSERT INTO workspaces (owner_user_id, name) SELECT id, $1 FROM u
			RETURNING id, owner_user_id
		), s AS (
			INSERT INTO skills (workspace_id, name) SELECT id, $1 FROM w
			RETURNING id, workspace_id
		), a AS (
			INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
			SELECT workspace_id, id, $1 || '-a', 'do the thing' FROM s
			RETURNING id
		), b AS (
			INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
			SELECT workspace_id, id, $1 || '-b', 'do the thing' FROM s
			RETURNING id, workspace_id
		), d AS (
			INSERT INTO datasets
				(workspace_id, test_case_id, file_name, content_type, size_bytes,
				 content_hash, object_key, expires_at)
			SELECT workspace_id, id, 'b.csv', 'text/csv', 3, 'hash-b',
			       'datasets/' || workspace_id || '/b', now() + interval '90 days'
			FROM b
			RETURNING id
		)
		SELECT w.id, w.owner_user_id, a.id, b.id, d.id FROM w, a, b, d`, tag).
		Scan(&ws.ID, &ws.OwnerUserID, &caseA, &caseB, &datasetB)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return ws, caseA, caseB, datasetB
}

func liveDatasets(t *testing.T, pool *pgxpool.Pool, datasetID pgtype.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM datasets WHERE id = $1 AND deleted_at IS NULL", datasetID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func datasetService(pool *pgxpool.Pool, store ObjectStore) *Service {
	return &Service{
		Pool: pool, Store: store,
		MayStoreObjects: (&identity.Service{}).MayStoreObjects,
	}
}

func TestDeleteDatasetRefusesAnUnrelatedParentInTheURL(t *testing.T) {
	pool := requireTestLabDB(t)
	store := &removedStore{}
	svc := datasetService(pool, store)
	ws, caseA, caseB, datasetB := seedTwoCases(t, pool)

	// The bug: B's file, addressed through A's URL.
	if _, err := svc.DeleteDataset(t.Context(), ws, caseA, datasetB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting through the wrong test case returned %v, want ErrNotFound", err)
	}
	if n := liveDatasets(t, pool, datasetB); n != 1 {
		t.Error("the file was soft-deleted through a test case that does not own it")
	}
	if len(store.removed) != 0 {
		t.Errorf("stored bytes were removed for a refused delete: %v", store.removed)
	}

	// The owner still works, so the guard is not simply refusing everything.
	if _, err := svc.DeleteDataset(t.Context(), ws, caseB, datasetB); err != nil {
		t.Fatalf("the owning test case could not delete its own file: %v", err)
	}
	if n := liveDatasets(t, pool, datasetB); n != 0 {
		t.Error("the owning test case's delete did not land")
	}
	if len(store.removed) != 1 {
		t.Errorf("the owner's delete removed %d objects, want 1", len(store.removed))
	}
}

func TestFailedDatasetObjectDeletionRemainsDurableRetryWork(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, _, caseB, datasetID := seedTwoCases(t, pool)
	store := &retryRemovalStore{err: errors.New("object store unavailable")}
	svc := datasetService(pool, store)

	ds, err := svc.DeleteDataset(t.Context(), ws, caseB, datasetID)
	if err != nil {
		t.Fatal(err)
	}
	var deleted, purged bool
	if err := pool.QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL, purged_at IS NOT NULL
		FROM datasets WHERE id = $1`, datasetID).Scan(&deleted, &purged); err != nil {
		t.Fatal(err)
	}
	if !deleted || purged {
		t.Fatalf("after failed removal: deleted=%v purged=%v, want hidden but retryable", deleted, purged)
	}
	var retryable bool
	if err := pool.QueryRow(t.Context(), `SELECT purged_at IS NULL AND
		(deleted_at IS NOT NULL OR expires_at <= now()) FROM datasets WHERE id = $1`, datasetID).Scan(&retryable); err != nil {
		t.Fatal(err)
	}
	if !retryable {
		t.Fatal("soft-deleted dataset disappeared from the durable cleanup predicate")
	}
	work, err := gen.New(pool).ListDatasetsPastRetention(t.Context(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, item := range work {
		if item.ID == datasetID {
			listed = true
			break
		}
	}
	if !listed {
		t.Fatal("soft-deleted dataset was not returned by the retention worklist")
	}

	store.err = nil
	candidate := objreconcile.Candidate{ID: ds.ID, WorkspaceID: ds.WorkspaceID, ObjectKey: ds.ObjectKey}
	n, err := objreconcile.PurgeExpired(t.Context(), pool, store,
		func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return []objreconcile.Candidate{candidate}, nil
		}, func(ctx context.Context, tx pgx.Tx, id pgtype.UUID) error {
			return gen.New(tx).MarkDatasetPurged(ctx, id)
		}, nil, 1)
	if err != nil || n != 1 {
		t.Fatalf("retry purge = %d, %v; want one completed row", n, err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT purged_at IS NOT NULL FROM datasets WHERE id = $1`, datasetID).Scan(&purged); err != nil {
		t.Fatal(err)
	}
	if !purged || store.removed != 2 {
		t.Fatalf("retry completion: purged=%v removal attempts=%d, want true and 2", purged, store.removed)
	}
}

func TestUploadDatasetDoesNotHoldTheTestCaseLockDuringObjectPut(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &blockingPutStore{started: make(chan struct{}), release: make(chan struct{})}
	svc := datasetService(pool, store)
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, err := svc.UploadDataset(context.Background(), ws, caseA, "rows.csv", []byte("id,name\n1,a\n"))
		done <- result{err: err}
	}()

	select {
	case <-store.started:
	case <-time.After(2 * time.Second):
		t.Fatal("object upload did not start")
	}
	var freshIntent int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM dataset_object_cleanup_intents
		WHERE workspace_id = $1 AND object_key = $2 AND not_before > now()`, ws.ID, store.key).Scan(&freshIntent); err != nil {
		t.Fatal(err)
	}
	if freshIntent != 1 {
		t.Fatal("upload did not publish a cleanup intent with a safety floor before object I/O")
	}
	due, err := gen.New(pool).ListDatasetCleanupIntents(t.Context(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range due {
		if row.ObjectKey == store.key {
			t.Fatal("maintenance claimed the cleanup intent of a still-running upload")
		}
	}
	updateCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	_, updateErr := pool.Exec(updateCtx, "UPDATE test_cases SET name = name WHERE id = $1", caseA)
	cancel()
	close(store.release)
	if updateErr != nil {
		t.Fatalf("database row stayed locked while object storage was blocked: %v", updateErr)
	}
	if got := <-done; got.err != nil {
		t.Fatalf("upload failed after storage was released: %v", got.err)
	}
	var datasets, intents int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM datasets WHERE workspace_id = $1 AND object_key = $2", ws.ID, store.key).Scan(&datasets); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM dataset_object_cleanup_intents WHERE workspace_id = $1 AND object_key = $2", ws.ID, store.key).Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if datasets != 1 || intents != 0 {
		t.Fatalf("successful upload left datasets=%d cleanup_intents=%d, want 1 and 0", datasets, intents)
	}
}

func TestUploadDatasetRemovesObjectAfterDefiniteQuotaFailure(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO datasets
			(workspace_id, test_case_id, file_name, content_type, size_bytes,
			 content_hash, object_key, expires_at)
		SELECT $1, $2, 'existing-' || n || '.csv', 'text/csv', 1,
		       'hash-' || n, 'datasets/existing/' || n, now() + interval '90 days'
		FROM generate_series(1, $3::int) AS n`, ws.ID, caseA, MaxFilesPerTestCase); err != nil {
		t.Fatalf("seed quota: %v", err)
	}
	store := &removedStore{}
	svc := datasetService(pool, store)
	if _, err := svc.UploadDataset(t.Context(), ws, caseA, "overflow.csv", []byte("id,name\n1,a\n")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("upload beyond file quota returned %v, want ErrLimitExceeded", err)
	}
	if len(store.removed) != 1 {
		t.Fatalf("definite quota failure removed %d objects, want 1", len(store.removed))
	}
}

func TestFailedUploadCompensationLeavesADurableCleanupIntent(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	if _, err := pool.Exec(t.Context(), `INSERT INTO datasets
		(workspace_id, test_case_id, file_name, content_type, size_bytes,
		 content_hash, object_key, expires_at)
		SELECT $1, $2, 'existing-' || n || '.csv', 'text/csv', 1,
		       'intent-hash-' || n, 'datasets/intent-existing/' || n,
		       now() + interval '90 days'
		FROM generate_series(1, $3::int) AS n`, ws.ID, caseA, MaxFilesPerTestCase); err != nil {
		t.Fatal(err)
	}
	store := &retryRemovalStore{err: errors.New("object store unavailable")}
	svc := datasetService(pool, store)
	if _, err := svc.UploadDataset(t.Context(), ws, caseA, "overflow.csv", []byte("id,name\n1,a\n")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("upload beyond quota returned %v, want ErrLimitExceeded", err)
	}
	var intentID pgtype.UUID
	if err := pool.QueryRow(t.Context(), `UPDATE dataset_object_cleanup_intents
		SET not_before = now() - interval '1 second'
		WHERE object_key = $1 RETURNING id`, store.key).Scan(&intentID); err != nil {
		t.Fatalf("failed compensation left no cleanup intent: %v", err)
	}
	rows, err := svc.DatasetCleanupIntentCandidates(t.Context(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	var candidate ReconcileCandidate
	for _, row := range rows {
		if row.ID == intentID {
			candidate = row
			break
		}
	}
	if !candidate.ID.Valid {
		t.Fatal("due upload cleanup intent was absent from the maintenance worklist")
	}
	store.err = nil
	n, err := objreconcile.PurgeExpired(t.Context(), pool, store,
		func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return []objreconcile.Candidate{{ID: candidate.ID, WorkspaceID: candidate.WorkspaceID, ObjectKey: candidate.ObjectKey}}, nil
		}, svc.MarkDatasetCleanupIntentPurged, svc.GuardDatasetObjectRemoval, 1)
	if err != nil || n != 1 {
		t.Fatalf("intent retry purge = %d, %v", n, err)
	}
	var remaining int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM dataset_object_cleanup_intents WHERE id = $1`, intentID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 || store.removed != 2 {
		t.Fatalf("cleanup intent remaining=%d removal attempts=%d, want 0 and 2", remaining, store.removed)
	}
}

func TestCleanupIntentCannotOvertakeALiveDatasetUpload(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	putStore := &blockingPutStore{started: make(chan struct{}), release: make(chan struct{})}
	svc := datasetService(pool, putStore)
	uploadDone := make(chan error, 1)
	go func() {
		_, err := svc.UploadDataset(t.Context(), ws, caseA, "slow.csv", []byte("id\n1\n"))
		uploadDone <- err
	}()
	<-putStore.started

	var candidate ReconcileCandidate
	if err := pool.QueryRow(t.Context(), `UPDATE dataset_object_cleanup_intents
		SET not_before = now() - interval '1 second'
		WHERE object_key = $1 RETURNING id, workspace_id, object_key`, putStore.key).
		Scan(&candidate.ID, &candidate.WorkspaceID, &candidate.ObjectKey); err != nil {
		t.Fatal(err)
	}
	cleanupStore := &retryRemovalStore{}
	cleanupDone := make(chan error, 1)
	go func() {
		_, err := objreconcile.PurgeExpired(t.Context(), pool, cleanupStore,
			func(context.Context, int32) ([]objreconcile.Candidate, error) {
				return []objreconcile.Candidate{{ID: candidate.ID, WorkspaceID: candidate.WorkspaceID, ObjectKey: candidate.ObjectKey}}, nil
			}, svc.MarkDatasetCleanupIntentPurged, svc.GuardDatasetObjectRemoval, 1)
		cleanupDone <- err
	}()
	select {
	case err := <-cleanupDone:
		t.Fatalf("cleanup overtook the live upload: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(putStore.release)
	if err := <-uploadDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	if cleanupStore.removed != 0 {
		t.Fatalf("cleanup removed %d live upload objects, want 0", cleanupStore.removed)
	}
}

func TestUploadDatasetCompensatesAfterCallerCancellation(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	ctx, cancel := context.WithCancel(context.Background())
	store := &cancelingPutStore{cancel: cancel}
	_, err := datasetService(pool, store).UploadDataset(
		ctx, ws, caseA, "rows.csv", []byte("id,name\n1,a\n"))
	if err == nil {
		t.Fatal("upload unexpectedly succeeded after its caller was cancelled")
	}
	if !store.removeCalled || store.removeCtxErr != nil {
		t.Fatalf("compensation did not get an independent live context: called=%v ctxErr=%v",
			store.removeCalled, store.removeCtxErr)
	}
}

func TestCommitCompensationOnlyRunsAfterADefiniteRollback(t *testing.T) {
	if !shouldCompensateCommit(pgx.ErrTxCommitRollback) {
		t.Fatal("definite rollback would retain an orphan object")
	}
	if shouldCompensateCommit(errors.New("connection lost during commit")) {
		t.Fatal("ambiguous commit error would delete bytes for a possibly committed row")
	}
	if shouldCompensateCommit(nil) {
		t.Fatal("successful commit requested compensation")
	}
}
