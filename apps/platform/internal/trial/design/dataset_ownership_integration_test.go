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

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

const testLabDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var testLabPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(testLabDBURLEnv)
	if dsn == "" {
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

func (s *removedStore) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	return nil
}

// seedTwoCases writes one workspace holding two test cases, with one file on the
// second. Raw SQL rather than the service: CreateTestCase needs Registry's skill
// reader and UploadDataset needs magic-byte-valid content, and neither is what
// this test is about.
func seedTwoCases(t *testing.T, pool *pgxpool.Pool) (ws identity.Workspace, caseA, caseB, datasetB pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	tag := strings.ReplaceAll(t.Name(), "/", "-")

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

func TestDeleteDatasetRefusesAnUnrelatedParentInTheURL(t *testing.T) {
	pool := requireTestLabDB(t)
	store := &removedStore{}
	svc := &Service{Pool: pool, Store: store}
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
