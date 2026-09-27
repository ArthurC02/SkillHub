package apiserver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type existsFailingStore struct {
	packageStore
	puts int
}

func (*existsFailingStore) Exists(context.Context, string) (bool, error) {
	return false, errors.New("simulated existence check failure")
}

func (s *existsFailingStore) Put(ctx context.Context, key string, data []byte) error {
	s.puts++
	return s.packageStore.Put(ctx, key, data)
}

func refuseArtifactRows(t *testing.T, pool *pgxpool.Pool, workspaceID string, atCommit bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION skillhub_test_refuse_artifact() RETURNS trigger AS $fn$
		BEGIN
			IF NEW.workspace_id = `+pgLiteral(workspaceID)+`::uuid THEN
				RAISE EXCEPTION 'artifact refused by test';
			END IF;
			RETURN NEW;
		END;
		$fn$ LANGUAGE plpgsql`); err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER skillhub_test_refuse_artifact BEFORE INSERT ON artifacts
		FOR EACH ROW EXECUTE FUNCTION skillhub_test_refuse_artifact()`
	if atCommit {
		trigger = `CREATE CONSTRAINT TRIGGER skillhub_test_refuse_artifact AFTER INSERT ON artifacts
			DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION skillhub_test_refuse_artifact()`
	}
	if _, err := pool.Exec(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TRIGGER IF EXISTS skillhub_test_refuse_artifact ON artifacts")
		_, _ = pool.Exec(ctx, "DROP FUNCTION IF EXISTS skillhub_test_refuse_artifact()")
	})
}

func workspaceDownloadObjects(a *api, workspaceID string) []string {
	var keys []string
	for key := range a.packages {
		if strings.HasPrefix(key, "downloads/"+workspaceID+"/") {
			keys = append(keys, key)
		}
	}
	return keys
}

func downloadIntentsIn(t *testing.T, pool *pgxpool.Pool, workspaceID string) int64 {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM download_object_cleanup_intents WHERE workspace_id = $1`,
		mustUUID(t, workspaceID))
}

func downloadObjectLockIsFree(t *testing.T, pool *pgxpool.Pool, objectKey string) bool {
	t.Helper()
	ctx := context.Background()
	probe, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close(ctx)
	var free bool
	if err := probe.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('artifact-object:' || $1::text, 0))`,
		objectKey).Scan(&free); err != nil {
		t.Fatal(err)
	}
	return free
}

func TestABuiltPackageLeavesNoCleanupIntentAndHoldsNoLock(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-built"))
	art := buildDownload(t, a, pool, c, uniqueWorklistLabel("package-built-skill"))
	key := "downloads/" + c.workspaceID + "/" + art.ContentHash + ".zip"

	if _, ok := a.packages[key]; !ok {
		t.Fatalf("no object at %s", key)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 0 {
		t.Errorf("%d cleanup intents outlived a committed package, want 0", n)
	}
	if !workspaceObjectsLockIsFree(t, pool, c.workspaceID) {
		t.Error("the workspace object lock is still held after packaging returned")
	}
	if !downloadObjectLockIsFree(t, pool, key) {
		t.Error("the download object lock is still held after packaging returned")
	}
}

func TestAPackageWhoseRowIsRefusedRemovesTheObjectItWroteAndItsIntent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-row-refused"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("package-row-refused-skill"))
	refuseArtifactRows(t, pool, c.workspaceID, false)

	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code < 500 {
		t.Fatalf("packaging with a refused row: got %d, want a server error", code)
	}
	if keys := workspaceDownloadObjects(a, c.workspaceID); len(keys) != 0 {
		t.Errorf("objects %v outlived a package that was never recorded", keys)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 0 {
		t.Errorf("%d cleanup intents remain after compensation, want 0", n)
	}
	if !workspaceObjectsLockIsFree(t, pool, c.workspaceID) {
		t.Error("the workspace object lock is still held after the failure")
	}
}

func TestAPackageFailingOverAnObjectItDidNotWriteLeavesThatObjectInPlace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-shared-object"))
	art := buildDownload(t, a, pool, c, uniqueWorklistLabel("package-shared-object-skill"))
	key := "downloads/" + c.workspaceID + "/" + art.ContentHash + ".zip"
	if _, err := pool.Exec(context.Background(), "UPDATE artifacts SET deleted_at = now() WHERE id = $1",
		mustUUID(t, art.ArtifactID)); err != nil {
		t.Fatal(err)
	}
	refuseArtifactRows(t, pool, c.workspaceID, false)

	if code, _ := postJSON(t, c, packagingPath(art.SkillID, art.SkillVersionID), `{"target":"standard"}`); code < 500 {
		t.Fatalf("repackaging with a refused row: got %d, want a server error", code)
	}
	if _, ok := a.packages[key]; !ok {
		t.Errorf("the object at %s, which this attempt found already there, was removed", key)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 1 {
		t.Errorf("%d cleanup intents, want the one this attempt recorded left for the sweep", n)
	}
}

func TestAPackageRefusedAtCommitKeepsItsObjectForTheCleanupIntent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-commit-refused"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("package-commit-refused-skill"))
	refuseArtifactRows(t, pool, c.workspaceID, true)

	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code < 500 {
		t.Fatalf("packaging refused at commit: got %d, want a server error", code)
	}
	if keys := workspaceDownloadObjects(a, c.workspaceID); len(keys) != 1 {
		t.Errorf("objects %v, want the written object kept for its cleanup intent", keys)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 1 {
		t.Errorf("%d cleanup intents cover the kept object, want 1", n)
	}
}

func TestAPackageWhoseObjectCannotBeCheckedWritesNothing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-exists-fails"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("package-exists-fails-skill"))
	store := &existsFailingStore{packageStore: a.packages}
	a.packaging.Store = store

	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code < 500 {
		t.Fatalf("packaging with an unreadable store: got %d, want a server error", code)
	}
	if store.puts != 0 {
		t.Errorf("%d objects written although the store could not say what it holds", store.puts)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 0 {
		t.Errorf("%d cleanup intents recorded, want 0", n)
	}
}
