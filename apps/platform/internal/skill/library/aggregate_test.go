package registry

// Skill Version aggregate invariants (ADR-003, iron rule 4; doc.go lists them).
//
// The pure ones are asserted with reflection, because the thing that must not
// happen is a *field appearing*, and no value-level assertion can express that.
// The rest need PostgreSQL: version numbering and duplicate rejection are an
// inline subquery and two unique indexes, and a mock would only restate the Go
// code that does not implement them.
//
// Point SKILLHUB_TEST_DATABASE_URL at a throwaway database and the database
// tests run; leave it unset and they skip, so CI without a database reports
// "skipped" rather than a false pass.
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.

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

// TestVersionNumberIsNotCallerSupplied pins invariant 2. Adding a VersionNumber
// field to either struct would compile, pass every other test, and quietly move
// the allocation from the query to whoever calls it — at which point two
// concurrent imports can agree on a number instead of one losing on
// skill_versions_number_key and retrying.
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

// TestNewVersionCarriesNoMutableState pins invariant 3 from the other side: the
// snapshot columns come from the validated package, so the struct must not grow
// a field that lets a caller hand-write one of them past validation.
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

// --- database invariants -----------------------------------------------------

const aggregateDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var aggregatePool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(aggregateDBURLEnv)
	if dsn == "" {
		os.Exit(m.Run()) // every database test skips; see requireRegistryDB
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

// validateDestructiveRegistryDatabaseURL refuses to point the schema drop below
// at anything that is not an obviously disposable local database.
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
		// No arguments means the simple protocol, so a file with several
		// statements applies as one batch.
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

// seedSkill writes the minimum chain a version hangs off. Raw SQL rather than
// internal/creator/workspace's service: what is under test is this aggregate's rules, and
// borrowing another context here would be a cross-context import bought for a
// fixture (ADR-032 §1).
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

// commitVersion runs the real write path, in its own transaction, the way ingest
// does. Returns the error so callers can assert on rejection.
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

// TestVersionNumberIsAllocatedByTheQuery is the half of invariant 2 that only a
// database can answer: the number comes from max()+1 over the skill, and the
// caller never sees the choice.
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

// TestIdenticalContentDoesNotBecomeASecondVersion pins SKILL-001/INGEST-005:
// re-saving the same bytes is not a new snapshot, and the rejection is
// skill_versions_content_key rather than a check the caller could forget.
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

// TestWrittenVersionRowIsFrozen is the aggregate's first invariant, and the
// point of asserting it here rather than trusting db/tests/immutability_test.sql
// is the path: this is the trigger firing on a row the production write path
// created, through the same pool the application uses.
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

// TestForkSharesThePackageObject pins invariant 5: a fork is rows, not bytes.
// Copying the object would double storage and, worse, produce a second content
// address for content the platform has already validated once.
func TestForkSharesThePackageObject(t *testing.T) {
	pool := requireRegistryDB(t)
	// Forked inside its own workspace: cross-workspace forking additionally
	// requires the source to be in the public catalog (WS-006), and that gate is
	// registry.go's business, not the aggregate's.
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
	// A fork starts its own numbering: it is a new skill, not a continuation.
	if forkVersion.VersionNumber != 1 {
		t.Errorf("fork version_number = %d, want 1", forkVersion.VersionNumber)
	}
	if fork.ForkedFromVersionID != origin.ID {
		t.Errorf("fork lineage = %v, want the origin version %v", fork.ForkedFromVersionID, origin.ID)
	}
	// The origin is untouched: forking reads it, and a read that wrote would be
	// the first crack in invariant 1.
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

// lockTestSchema serialises the packages that reset this database.
//
// apiserver, eval and registry each drop and recreate schema "public" in
// SKILLHUB_TEST_DATABASE_URL, and `go test ./...` runs packages concurrently:
// one package's reset lands while another is mid-run, and the second one sees
// "relation does not exist". Held on one connection for the whole package run
// rather than only across the migration, because the hazard is a reset
// colliding with somebody else's *tests*, not with their migration.
//
// Session-scoped, so a crashed run releases it along with its connection and a
// stale lock cannot wedge CI. Every package that resets this database must take
// it; one that forgets fails loudly with the panic above rather than silently.
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
