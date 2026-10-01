package testlab

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type recordingStore struct {
	puts      []string
	removed   []string
	refusePut bool
}

func (s *recordingStore) PutFrom(_ context.Context, key string, _ io.Reader, _ int64) error {
	s.puts = append(s.puts, key)
	if s.refusePut {
		return errors.New("simulated object write failure")
	}
	return nil
}

func (*recordingStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}

func (s *recordingStore) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	return nil
}

func seedDatasetBytes(t *testing.T, pool *pgxpool.Pool, ws identity.Workspace, testCase pgtype.UUID, size int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO datasets
		(workspace_id, test_case_id, file_name, content_type, size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'bulk.csv', 'text/csv', $3, 'bulk-hash', 'datasets/bulk/' || gen_random_uuid(), now() + interval '90 days')`,
		ws.ID, testCase, size); err != nil {
		t.Fatal(err)
	}
}

func cleanupIntentsFor(t *testing.T, pool *pgxpool.Pool, key string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dataset_object_cleanup_intents WHERE object_key = $1`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func workspaceIsFree(t *testing.T, workspaceID pgtype.UUID) bool {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), os.Getenv(testLabDBURLEnv))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var taken bool
	if err := conn.QueryRow(t.Context(),
		`SELECT pg_try_advisory_lock(hashtextextended('workspace-objects:' || $1::uuid::text, 0))`,
		workspaceID).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	return taken
}

func refuseCommitOf(t *testing.T, pool *pgxpool.Pool, fileName string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION skillhub_test_refuse_dataset_commit() RETURNS trigger AS $fn$
		BEGIN
			IF NEW.file_name = '`+fileName+`' THEN
				RAISE EXCEPTION 'refused at commit';
			END IF;
			RETURN NULL;
		END
		$fn$ LANGUAGE plpgsql;
		CREATE CONSTRAINT TRIGGER skillhub_test_refuse_dataset_commit
			AFTER INSERT ON datasets DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW EXECUTE FUNCTION skillhub_test_refuse_dataset_commit();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS skillhub_test_refuse_dataset_commit ON datasets;
			DROP FUNCTION IF EXISTS skillhub_test_refuse_dataset_commit();`)
	})
}

func TestUploadDatasetRefusesAnUnusableFileBeforeAnyStorage(t *testing.T) {
	store := &recordingStore{}
	svc := &Service{Store: store}
	for name, tc := range map[string]struct {
		fileName string
		data     []byte
		want     error
	}{
		"no file name":         {"  ", []byte("id\n1\n"), ErrInvalid},
		"a name that is a dot": {"..", []byte("id\n1\n"), ErrInvalid},
		"an empty file":        {"rows.csv", nil, ErrInvalid},
		"one byte over a file": {"rows.csv", bytes.Repeat([]byte("a"), MaxFileBytes+1), ErrLimitExceeded},
		"an executable":        {"tool.exe", []byte("MZ\x90\x00rest"), ErrUnsupportedType},
	} {
		if _, err := svc.UploadDataset(context.Background(), identity.Workspace{}, pgtype.UUID{}, tc.fileName, bytes.NewReader(tc.data)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if len(store.puts) != 0 {
		t.Errorf("refused files reached storage: %v", store.puts)
	}
}

func TestUploadDatasetOfExactlyTheFileLimitIsStored(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{}

	ds, err := datasetService(pool, store).UploadDataset(t.Context(), ws, caseA, "big.csv", bytes.NewReader(bytes.Repeat([]byte("a"), MaxFileBytes)))
	if err != nil || ds.SizeBytes != MaxFileBytes || len(store.puts) != 1 {
		t.Fatalf("size=%d puts=%d err=%v, want one stored file of exactly the limit", ds.SizeBytes, len(store.puts), err)
	}
}

func TestUploadDatasetToAnotherWorkspacesTestCaseTouchesNoStorage(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, _, _, _ := seedTwoCases(t, pool)
	_, foreignCase, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{}

	if _, err := datasetService(pool, store).UploadDataset(t.Context(), ws, foreignCase, "rows.csv", bytes.NewReader([]byte("id\n1\n"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(store.puts) != 0 || len(store.removed) != 0 {
		t.Errorf("puts=%v removed=%v, want storage untouched", store.puts, store.removed)
	}
}

func TestUploadDatasetToAWorkspaceThatMayNotStoreObjectsIsNotFoundAndLetsGo(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{}
	svc := datasetService(pool, store)
	svc.MayStoreObjects = func(context.Context, gen.DBTX, pgtype.UUID) (bool, error) { return false, nil }

	if _, err := svc.UploadDataset(t.Context(), ws, caseA, "rows.csv", bytes.NewReader([]byte("id\n1\n"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(store.puts) != 0 {
		t.Errorf("a workspace that may not store objects reached storage: %v", store.puts)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the refused upload kept the workspace held")
	}
}

func TestUploadDatasetWithoutALifecycleReadRefusesAndLetsGo(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{}
	svc := &Service{Pool: pool, Store: store}

	_, err := svc.UploadDataset(t.Context(), ws, caseA, "rows.csv", bytes.NewReader([]byte("id\n1\n")))
	if err == nil || errors.Is(err, ErrNotFound) || len(store.puts) != 0 {
		t.Fatalf("err=%v puts=%v, want a configuration error before storage", err, store.puts)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the refused upload kept the workspace held")
	}
}

func TestUploadDatasetAtExactlyTheTestCaseByteLimitIsStoredAndOneByteOverIsNot(t *testing.T) {
	pool := requireTestLabDB(t)
	data := []byte("id\n1\n")

	atLimit, caseAtLimit, _, _ := seedTwoCases(t, pool)
	seedDatasetBytes(t, pool, atLimit, caseAtLimit, MaxTestCaseBytes-int64(len(data)))
	if _, err := datasetService(pool, &recordingStore{}).UploadDataset(t.Context(), atLimit, caseAtLimit, "fits.csv", bytes.NewReader(data)); err != nil {
		t.Errorf("an upload that fills the test case exactly: %v, want success", err)
	}

	over, caseOver, _, _ := seedTwoCases(t, pool)
	seedDatasetBytes(t, pool, over, caseOver, MaxTestCaseBytes-int64(len(data))+1)
	store := &recordingStore{}
	if _, err := datasetService(pool, store).UploadDataset(t.Context(), over, caseOver, "over.csv", bytes.NewReader(data)); !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("one byte over the test case: %v, want ErrLimitExceeded", err)
	}
	if len(store.puts) != 1 || len(store.removed) != 1 || store.removed[0] != store.puts[0] {
		t.Errorf("puts=%v removed=%v, want the written object removed again", store.puts, store.removed)
	}
}

func TestACompensatedUploadLeavesNoCleanupIntentAndLetsGo(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	seedDatasetBytes(t, pool, ws, caseA, MaxTestCaseBytes)
	store := &recordingStore{}

	if _, err := datasetService(pool, store).UploadDataset(t.Context(), ws, caseA, "over.csv", bytes.NewReader([]byte("id\n1\n"))); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
	if len(store.removed) != 1 {
		t.Fatalf("removed %v, want the one written object", store.removed)
	}
	if n := cleanupIntentsFor(t, pool, store.removed[0]); n != 0 {
		t.Errorf("%d cleanup intents remain for an object that was removed, want 0", n)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the compensated upload kept the workspace held")
	}
}

func TestAnUploadWhoseObjectWriteFailsRemovesWhatItMayHaveWrittenAndItsIntent(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{refusePut: true}

	_, err := datasetService(pool, store).UploadDataset(t.Context(), ws, caseA, "rows.csv", bytes.NewReader([]byte("id\n1\n")))
	if err == nil || err.Error() != "simulated object write failure" {
		t.Fatalf("err = %v, want the store's own failure", err)
	}
	if len(store.puts) != 1 || len(store.removed) != 1 || store.removed[0] != store.puts[0] {
		t.Fatalf("puts=%v removed=%v, want the attempted key removed", store.puts, store.removed)
	}
	if n := cleanupIntentsFor(t, pool, store.puts[0]); n != 0 {
		t.Errorf("%d cleanup intents remain after compensation, want 0", n)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the failed upload kept the workspace held")
	}
}

func TestAStoredUploadIsRecordedUnderItsWorkspaceWithTheFileItWasGiven(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	store := &recordingStore{}
	data := []byte("id,name\n1,a\n")

	ds, err := datasetService(pool, store).UploadDataset(t.Context(), ws, caseA, `dir\rows.csv`, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if ds.FileName != "rows.csv" || ds.ContentType != "text/plain" || ds.SizeBytes != int64(len(data)) ||
		ds.WorkspaceID != ws.ID || ds.TestCaseID != caseA || len(store.puts) != 1 || ds.ObjectKey != store.puts[0] {
		t.Errorf("dataset = %+v puts=%v, want rows.csv as text/plain of %d bytes stored at the written key", ds, store.puts, len(data))
	}
	if n := cleanupIntentsFor(t, pool, ds.ObjectKey); n != 0 || len(store.removed) != 0 {
		t.Errorf("a stored upload left %d cleanup intents and removed %v", n, store.removed)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the stored upload kept the workspace held")
	}
}

func TestAnUploadWhoseCommitFailsWithoutAVerdictKeepsItsBytesForTheCleanupIntent(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, caseA, _, _ := seedTwoCases(t, pool)
	refuseCommitOf(t, pool, "refused-at-commit.csv")
	store := &recordingStore{}

	_, err := datasetService(pool, store).UploadDataset(t.Context(), ws, caseA, "refused-at-commit.csv", bytes.NewReader([]byte("id\n1\n")))
	if err == nil || errors.Is(err, pgx.ErrTxCommitRollback) {
		t.Fatalf("err = %v, want the commit's own error", err)
	}
	if len(store.puts) != 1 || len(store.removed) != 0 {
		t.Fatalf("puts=%v removed=%v, want the written object left in place", store.puts, store.removed)
	}
	if n := cleanupIntentsFor(t, pool, store.puts[0]); n != 1 {
		t.Errorf("%d cleanup intents cover the kept object, want 1", n)
	}
	if !workspaceIsFree(t, ws.ID) {
		t.Error("the failed upload kept the workspace held")
	}
}
