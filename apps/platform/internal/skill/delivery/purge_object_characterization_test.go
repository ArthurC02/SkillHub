package packaging

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var purgePool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintln(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset; this run would have skipped every database test and still reported success")
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	schemaLock, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := schemaLock.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	purgePool = pool
	code := m.Run()
	_, _ = schemaLock.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
	schemaLock.Release()
	pool.Close()
	os.Exit(code)
}

type removalRecorder struct {
	ObjectStore
	removed []string
	fail    error
}

func (r *removalRecorder) Remove(_ context.Context, key string) error {
	r.removed = append(r.removed, key)
	return r.fail
}

func purgeTestConn(t *testing.T) *pgxpool.Conn {
	t.Helper()
	if purgePool == nil {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the download purge database test")
	}
	conn, err := purgePool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	return conn
}

func unsharedObjectKey(t *testing.T) string {
	t.Helper()
	return "downloads/purge-characterization/" + rand.Text() + ".zip"
}

func TestADeletedDownloadWhoseObjectNoLiveArtifactSharesIsRemovedAndMarkedPurged(t *testing.T) {
	conn := purgeTestConn(t)
	store := &removalRecorder{}
	var cleared []pgtype.UUID
	svc := &Service{Store: store, ClearSightings: func(_ context.Context, _ pgx.Tx, ids []pgtype.UUID) error {
		cleared = append(cleared, ids...)
		return nil
	}}
	row := gen.SoftDeleteDownloadArtifactRow{ID: pgtype.UUID{Bytes: [16]byte{0xde, 0x1e}, Valid: true}, ObjectKey: unsharedObjectKey(t)}

	svc.purgeDeletedDownloadObject(context.Background(), conn, row)

	if len(store.removed) != 1 || store.removed[0] != row.ObjectKey {
		t.Fatalf("removed %v, want exactly the deleted download's object %q", store.removed, row.ObjectKey)
	}
	if len(cleared) != 1 || cleared[0] != row.ID {
		t.Fatalf("purge marked %v, want the deleted artifact %v", cleared, row.ID)
	}
}

func TestADownloadObjectThatCouldNotBeRemovedIsNotMarkedPurged(t *testing.T) {
	conn := purgeTestConn(t)
	store := &removalRecorder{fail: errors.New("object store unavailable")}
	marked := false
	svc := &Service{Store: store, ClearSightings: func(context.Context, pgx.Tx, []pgtype.UUID) error {
		marked = true
		return nil
	}}
	row := gen.SoftDeleteDownloadArtifactRow{ID: pgtype.UUID{Bytes: [16]byte{0xde, 0x1f}, Valid: true}, ObjectKey: unsharedObjectKey(t)}

	svc.purgeDeletedDownloadObject(context.Background(), conn, row)

	if len(store.removed) != 1 {
		t.Fatalf("removal attempts = %v, want one", store.removed)
	}
	if marked {
		t.Fatal("a download whose object is still in the store was marked purged; cleanup would never retry it")
	}
}
