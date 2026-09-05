package ingest

// GEN-010: confirming the same generated candidate twice must not error. This
// exercises the real database because the duplicate verdict comes from
// persistVersion's VersionByContent lookup (a unique index), and
// beginPackageWrite takes real advisory locks against a real workspace row —
// nothing here is a restatement of Go code that could be faked instead.
//
// Point SKILLHUB_TEST_DATABASE_URL at a throwaway database and this test runs;
// leave it unset and it skips, so CI without a database reports "skipped"
// rather than a false pass (02:PORT-004).
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

const creationDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var creationPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(creationDBURLEnv)
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", creationDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run()) // every database test skips; see requireCreationDB
	}
	if err := validateDestructiveCreationDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockCreationTestSchema(ctx, pool)
	if err := migrateCreationSchema(ctx, pool); err != nil {
		panic(err)
	}
	creationPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

// validateDestructiveCreationDatabaseURL refuses to point the schema drop
// below at anything that is not an obviously disposable local database.
func validateDestructiveCreationDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", creationDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", creationDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", creationDBURLEnv)
	}
	return nil
}

func migrateCreationSchema(ctx context.Context, pool *pgxpool.Pool) error {
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
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// lockCreationTestSchema serialises the packages that reset this database.
// Same lock name skill/library and trial/improvement already take (see their
// aggregate_test.go), so a concurrent `go test ./...` run does not see one
// package's reset land mid another package's test.
func lockCreationTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
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

func requireCreationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if creationPool == nil {
		t.Skipf("%s not set; skipping MaterializeGeneratedCandidate database test", creationDBURLEnv)
	}
	return creationPool
}

// seedCreationWorkspace writes the minimum row MaterializeGeneratedCandidate's
// object-write fence needs (identity.LockObjectWrite checks a real workspace
// row). Raw SQL rather than internal/creator/workspace's service: borrowing
// another context's writer for a fixture here would be a cross-context import
// bought for a test (ADR-032 §1) — the same reason skill/library's own
// seedSkill helper does the same thing.
func seedCreationWorkspace(t *testing.T, pool *pgxpool.Pool, name string) identity.Workspace {
	t.Helper()
	ctx := context.Background()
	var ws identity.Workspace
	var userID pgtype.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name) VALUES ($1, $1) RETURNING id`,
		name+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO workspaces (owner_user_id, name) VALUES ($1, $2)
		 RETURNING id, owner_user_id, name, created_at, updated_at, is_catalog`,
		userID, name).Scan(&ws.ID, &ws.OwnerUserID, &ws.Name, &ws.CreatedAt, &ws.UpdatedAt, &ws.IsCatalog); err != nil {
		t.Fatal(err)
	}
	return ws
}

// creationTestStore is an in-memory ObjectStore: package bytes for this test
// never need to touch real object storage, only the fence around them does.
type creationTestStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *creationTestStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *creationTestStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key], nil
}

// TestCreationCandidateMaterializeDuplicateIsReuseNotError pins GEN-010's
// 「同一候選重複確認不可重複建版」: confirming byte-identical content a second
// time against the same generated skill must return the version already on
// record, not the old hard error that turned a no-op into a 503.
func TestCreationCandidateMaterializeDuplicateIsReuseNotError(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "creation-dup-fixture")

	svc := &Service{
		Pool:  pool,
		Store: &creationTestStore{},
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error {
			return nil // the search projection's shape is not under test here
		},
	}

	skill := goodGeneratedSkill()
	prov := GeneratedCandidateProvenance{TaskDescription: "test task", Model: "test-model", PromptVersion: "v1"}

	first, err := svc.MaterializeGeneratedCandidate(ctx, ws, skill, prov, nil)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first materialize reported Duplicate; nothing existed yet")
	}

	prov.ExistingSkillID = &first.Skill.ID
	second, err := svc.MaterializeGeneratedCandidate(ctx, ws, skill, prov, nil)
	if err != nil {
		t.Fatalf("second materialize of identical content returned an error instead of reusing the version: %v", err)
	}
	if !second.Duplicate {
		t.Error("second materialize of identical content was not reported as a duplicate")
	}
	if second.Version.ID != first.Version.ID {
		t.Errorf("second materialize Version.ID = %v, want the existing version %v", second.Version.ID, first.Version.ID)
	}
	if second.Skill.ID != first.Skill.ID {
		t.Errorf("second materialize Skill.ID = %v, want %v", second.Skill.ID, first.Skill.ID)
	}
}
