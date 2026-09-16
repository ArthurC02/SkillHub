package registry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
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
				t.Errorf("%s.%s: version_number is allocated by CreateSkillVersion, never by the caller",
					typ.Name(), name)
			}
		}
	}
}

func TestNewVersionCarriesNoMutableState(t *testing.T) {
	want := map[string]bool{
		"SourceID":    true,
		"ContentHash": true, "PackageObjectKey": true, "Report": true,
	}
	typ := reflect.TypeOf(NewVersion{})
	for i := range typ.NumField() {
		if name := typ.Field(i).Name; !want[name] {
			t.Errorf("NewVersion.%s is new; a version row is a snapshot of the Report, "+
				"so check it cannot be set independently of validation", name)
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

func commitVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID pgtype.UUID, v NewVersion) (Version, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root, err := LoadSkill(ctx, tx, workspaceID, skillID)
	if err != nil {
		return Version{}, err
	}
	content, err := ContentFromPackage(v, false)
	if err != nil {
		return Version{}, err
	}
	root.AddVersion(content)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return Version{}, err
	}
	return root.AddedVersion(), tx.Commit(ctx)
}

func TestVersionNumberIsAllocatedByTheQuery(t *testing.T) {
	pool := requireRegistryDB(t)
	ws, skillID := seedSkill(t, pool, "numbering")

	for i, hash := range []string{"hash-a", "hash-b", "hash-c"} {
		version, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
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
		ContentHash:      "same-bytes",
		PackageObjectKey: "packages/same-bytes",
		Report:           passingReport("duplicate"),
	}
	if _, err := commitVersion(t, pool, ws.ID, skillID, v); err != nil {
		t.Fatal(err)
	}
	if _, err := commitVersion(t, pool, ws.ID, skillID, v); !isUniqueViolation(err) {
		t.Fatalf("second insert of identical content: err = %v, want a unique violation", err)
	}
}

func TestWrittenVersionRowIsFrozen(t *testing.T) {
	pool := requireRegistryDB(t)
	ws, skillID := seedSkill(t, pool, "frozen")
	version, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
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
	origin, err := commitVersion(t, pool, ws.ID, sourceSkill, NewVersion{
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

func TestEverySkillGovernanceCommandLeavesItsEventInTheOutbox(t *testing.T) {
	pool := requireRegistryDB(t)
	row, skillID := seedSkill(t, pool, "governance-events")
	ws := identity.Workspace{ID: row.ID, OwnerUserID: row.OwnerUserID}
	ctx := context.Background()
	s := testProjection(&Service{Pool: pool})
	s.RefreshListing = func(context.Context, gen.DBTX, pgtype.UUID) error { return nil }
	inTx := func(write func(tx pgx.Tx) error) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := write(tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	documents := CategoryDocuments
	if _, err := s.SetCategory(ctx, ws, skillID, &documents); err != nil {
		t.Fatalf("categorize: %v", err)
	}
	held := "license-review"
	inTx(func(tx pgx.Tx) error { _, err := SetAccessRestriction(ctx, tx, skillID, &held); return err })
	inTx(func(tx pgx.Tx) error { _, err := SetAccessRestriction(ctx, tx, skillID, nil); return err })
	inTx(func(tx pgx.Tx) error {
		_, err := SetRedistribution(ctx, tx, skillID, string(RedistributionBlocked), LicenseClaim{})
		return err
	})
	if _, err := s.Takedown(ctx, ws, skillID, "the licence was withdrawn"); err != nil {
		t.Fatalf("take down: %v", err)
	}
	if _, err := s.Takedown(ctx, ws, skillID, "a second report"); !errors.Is(err, ErrAlreadyTakenDown) {
		t.Fatalf("second takedown: want ErrAlreadyTakenDown, got %v", err)
	}
	if _, err := s.Delete(ctx, ws, skillID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT event_type FROM outbox_events
		WHERE aggregate_type = 'skill' AND aggregate_id = $1 AND correlation_id = $1`, skillID)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	want := []string{
		"skill.categorized", "skill.access_restricted", "skill.access_restriction_lifted",
		"skill.redistribution_set", "skill.taken_down", "skill.deleted",
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outbox events:\n got  %v\n want %v", got, want)
	}
}

func TestGovernanceCommandsOnASkillTheyCannotSeeAnswerNotFound(t *testing.T) {
	pool := requireRegistryDB(t)
	row, _ := seedSkill(t, pool, "governance-scope-a")
	_, elsewhere := seedSkill(t, pool, "governance-scope-b")
	ws := identity.Workspace{ID: row.ID, OwnerUserID: row.OwnerUserID}
	missing := pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}
	ctx := context.Background()
	s := testProjection(&Service{Pool: pool})
	s.RefreshListing = func(context.Context, gen.DBTX, pgtype.UUID) error { return nil }
	inTx := func(write func(tx pgx.Tx) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		return write(tx)
	}
	documents := CategoryDocuments
	held := "license-review"

	for name, err := range map[string]error{
		"take down another workspace's skill": func() error { _, err := s.Takedown(ctx, ws, elsewhere, "reason"); return err }(),
		"delete another workspace's skill":    func() error { _, err := s.Delete(ctx, ws, elsewhere); return err }(),
		"categorize another workspace's skill": func() error {
			_, err := s.SetCategory(ctx, ws, elsewhere, &documents)
			return err
		}(),
		"restrict a skill that does not exist": inTx(func(tx pgx.Tx) error {
			_, err := SetAccessRestriction(ctx, tx, missing, &held)
			return err
		}),
		"set the redistribution of a skill that does not exist": inTx(func(tx pgx.Tx) error {
			_, err := SetRedistribution(ctx, tx, missing, string(RedistributionBlocked), LicenseClaim{})
			return err
		}),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: want ErrNotFound, got %v", name, err)
		}
	}
}

func TestEveryVersionPathLeavesItsEventsInTheOutbox(t *testing.T) {
	pool := requireRegistryDB(t)
	row, _ := seedSkill(t, pool, "lifecycle-events")
	ws := identity.Workspace{ID: row.ID, OwnerUserID: row.OwnerUserID}
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	imported, err := SkillFromPackage(ws.ID, passingReport("lifecycle-imported"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSkill(ctx, tx, imported); err != nil {
		t.Fatalf("create: %v", err)
	}
	var sourceID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO skill_sources (workspace_id, source_type, content_hash, fetched_at)
		VALUES ($1, 'upload', 'lifecycle-bytes', now()) RETURNING id`, ws.ID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	newer := passingReport("lifecycle-imported")
	newer.Manifest.Description = "a newer summary"
	content, err := ContentFromPackage(NewVersion{
		SourceID: sourceID, ContentHash: "lifecycle-bytes", PackageObjectKey: "packages/lifecycle-bytes",
		Report: newer,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	imported.AddVersion(content)
	imported.AdoptNewestSummary()
	if err := SaveSkill(ctx, tx, imported); err != nil {
		t.Fatalf("add version: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	fork, _, err := testProjection(&Service{Pool: pool}).Fork(ctx, ws, imported.ID())
	if err != nil {
		t.Fatalf("fork: %v", err)
	}

	for skill, want := range map[pgtype.UUID][]string{
		imported.ID(): {"skill.created", "skill.described", "skill.version_added"},
		fork.ID:       {"skill.created", "skill.version_added"},
	} {
		rows, err := pool.Query(ctx, `
			SELECT event_type FROM outbox_events
			WHERE aggregate_type = 'skill' AND aggregate_id = $1 AND correlation_id = $1
			ORDER BY event_type`, skill)
		if err != nil {
			t.Fatalf("read outbox: %v", err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("read outbox: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("outbox events of %v:\n got  %v\n want %v", skill, got, want)
		}
	}

	var summary, versionID, announced string
	var versionSource pgtype.UUID
	if err := pool.QueryRow(ctx, `
		SELECT s.summary, v.id::text, v.source_id, o.payload->>'version_id'
		FROM skills s
		JOIN skill_versions v ON v.skill_id = s.id
		JOIN outbox_events o ON o.aggregate_id = s.id AND o.event_type = 'skill.version_added'
		WHERE s.id = $1`, imported.ID()).Scan(&summary, &versionID, &versionSource, &announced); err != nil {
		t.Fatal(err)
	}
	if summary != "a newer summary" || versionSource != sourceID || announced != versionID {
		t.Errorf("summary %q, version source %v, announced version %q; want the newer summary, source %v and version %q",
			summary, versionSource, announced, sourceID, versionID)
	}
}
