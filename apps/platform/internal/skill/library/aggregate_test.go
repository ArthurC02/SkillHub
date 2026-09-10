package registry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestVersionNumberIsNotCallerSupplied(t *testing.T) {
	for _, subject := range []any{NewVersion{}, gen.CreateSkillVersionParams{}} {
		typ := reflect.TypeOf(subject)
		for i := range typ.NumField() {
			if name := typ.Field(i).Name; strings.Contains(strings.ToLower(name), "version") &&
				strings.Contains(strings.ToLower(name), "number") {
				t.Errorf("%s.%s: version_number is allocated by CreateSkillVersion, never by the caller (ADR-003)",
					typ.Name(), name)
			}
		}
	}
}

func TestNewVersionCarriesNoMutableState(t *testing.T) {
	want := map[string]bool{
		"WorkspaceID": true, "SkillID": true, "SourceID": true,
		"ContentHash": true, "PackageObjectKey": true, "Report": true,
	}
	typ := reflect.TypeOf(NewVersion{})
	for i := range typ.NumField() {
		if name := typ.Field(i).Name; !want[name] {
			t.Errorf("NewVersion.%s is new; a version row is a snapshot of the Report, "+
				"so check it cannot be set independently of validation (doc.go invariant 3)", name)
		}
		delete(want, typ.Field(i).Name)
	}
	for name := range want {
		t.Errorf("NewVersion.%s disappeared; update this test with the reason", name)
	}
}

const aggregateDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var aggregatePool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(aggregateDBURLEnv)
	if dsn == "" {

		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", aggregateDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	if err := validateDestructiveRegistryDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migrateRegistrySchema(ctx, pool); err != nil {
		panic(err)
	}
	aggregatePool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

func validateDestructiveRegistryDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", aggregateDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", aggregateDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", aggregateDBURLEnv)
	}
	return nil
}

func TestDestructiveRegistryDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
	} {
		if err := validateDestructiveRegistryDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructiveRegistryDatabaseURL("postgres://u:p@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func migrateRegistrySchema(ctx context.Context, pool *pgxpool.Pool) error {
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

		// Exec with no arguments uses the simple protocol, which applies a
		// multi-statement string as one batch.
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func requireRegistryDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if aggregatePool == nil {
		t.Skipf("%s not set; skipping Skill Version aggregate invariant test", aggregateDBURLEnv)
	}
	return aggregatePool
}

func seedSkill(t *testing.T, pool *pgxpool.Pool, name string) (ws gen.Workspace, skillID pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
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
	if err := pool.QueryRow(ctx,
		`INSERT INTO skills (workspace_id, name) VALUES ($1, $2) RETURNING id`,
		ws.ID, name).Scan(&skillID); err != nil {
		t.Fatal(err)
	}
	return ws, skillID
}

func passingReport(name string) skillpkg.Report {
	return skillpkg.Report{Manifest: &skillpkg.Manifest{Name: name, Description: "fixture"}}
}

func commitVersion(t *testing.T, pool *pgxpool.Pool, v NewVersion) (Version, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	version, err := CreateVersionFromPackage(ctx, tx, v)
	if err != nil {
		return Version{}, err
	}
	return version, tx.Commit(ctx)
}

func TestVersionNumberIsAllocatedByTheQuery(t *testing.T) {
	pool := requireRegistryDB(t)
	ws, skillID := seedSkill(t, pool, "numbering")

	for i, hash := range []string{"hash-a", "hash-b", "hash-c"} {
		version, err := commitVersion(t, pool, NewVersion{
			WorkspaceID:      ws.ID,
			SkillID:          skillID,
			ContentHash:      hash,
			PackageObjectKey: "packages/" + hash,
			Report:           passingReport("numbering"),
		})
		if err != nil {
			t.Fatalf("version %d: %v", i+1, err)
		}
		if int(version.VersionNumber) != i+1 {
			t.Fatalf("version_number = %d, want %d", version.VersionNumber, i+1)
		}
	}
}

func TestIdenticalContentDoesNotBecomeASecondVersion(t *testing.T) {
	pool := requireRegistryDB(t)
	ws, skillID := seedSkill(t, pool, "duplicate")
	v := NewVersion{
		WorkspaceID:      ws.ID,
		SkillID:          skillID,
		ContentHash:      "same-bytes",
		PackageObjectKey: "packages/same-bytes",
		Report:           passingReport("duplicate"),
	}
	if _, err := commitVersion(t, pool, v); err != nil {
		t.Fatal(err)
	}
	if _, err := commitVersion(t, pool, v); !isUniqueViolation(err) {
		t.Fatalf("second insert of identical content: err = %v, want a unique violation", err)
	}
}

func TestWrittenVersionRowIsFrozen(t *testing.T) {
	pool := requireRegistryDB(t)
	ws, skillID := seedSkill(t, pool, "frozen")
	version, err := commitVersion(t, pool, NewVersion{
		WorkspaceID:      ws.ID,
		SkillID:          skillID,
		ContentHash:      "frozen-bytes",
		PackageObjectKey: "packages/frozen-bytes",
		Report:           passingReport("frozen"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, statement := range map[string]string{
		"update": `UPDATE skill_versions SET content_hash = 'tampered' WHERE id = $1`,
		"delete": `DELETE FROM skill_versions WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, version.ID); err == nil {
			t.Errorf("%s of a written version succeeded; iron rule 4 says it must not", name)
		} else if !strings.Contains(err.Error(), "immutable") {
			t.Errorf("%s failed for the wrong reason: %v", name, err)
		}
	}
}

func TestForkSharesThePackageObject(t *testing.T) {
	pool := requireRegistryDB(t)

	ws, sourceSkill := seedSkill(t, pool, "fork-source")
	origin, err := commitVersion(t, pool, NewVersion{
		WorkspaceID:      ws.ID,
		SkillID:          sourceSkill,
		ContentHash:      "shared-bytes",
		PackageObjectKey: "packages/shared-bytes",
		Report:           passingReport("fork-source"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	service := testProjection(&Service{Pool: pool})
	fork, forkVersion, err := service.Fork(ctx, identity.Workspace{
		ID: ws.ID, OwnerUserID: ws.OwnerUserID, Name: ws.Name,
		CreatedAt: ws.CreatedAt, UpdatedAt: ws.UpdatedAt, IsCatalog: ws.IsCatalog,
	}, sourceSkill)
	if err != nil {
		t.Fatal(err)
	}
	if forkVersion.PackageObjectKey != origin.PackageObjectKey {
		t.Errorf("fork object key = %q, want the shared %q", forkVersion.PackageObjectKey, origin.PackageObjectKey)
	}
	if forkVersion.ContentHash != origin.ContentHash {
		t.Errorf("fork content hash = %q, want %q", forkVersion.ContentHash, origin.ContentHash)
	}

	if forkVersion.VersionNumber != 1 {
		t.Errorf("fork version_number = %d, want 1", forkVersion.VersionNumber)
	}
	if fork.ForkedFromVersionID != origin.ID {
		t.Errorf("fork lineage = %v, want the origin version %v", fork.ForkedFromVersionID, origin.ID)
	}

	var after gen.SkillVersion
	if err := pool.QueryRow(ctx,
		`SELECT content_hash, version_number FROM skill_versions WHERE id = $1`, origin.ID).
		Scan(&after.ContentHash, &after.VersionNumber); err != nil {
		t.Fatal(err)
	}
	if after.ContentHash != origin.ContentHash || after.VersionNumber != origin.VersionNumber {
		t.Errorf("origin version changed during fork: %+v", after)
	}
}

// lockTestSchema holds a session-scoped Postgres advisory lock on one
// dedicated connection, so concurrent test binaries resetting this schema
// serialize instead of racing.
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
