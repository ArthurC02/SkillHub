package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func purgeDatasets(t *testing.T, pool *pgxpool.Pool, store objreconcile.ObjectStore) int {
	t.Helper()
	svc := &testlab.Service{Pool: pool}
	n, err := objreconcile.PurgeExpired(context.Background(), pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ExpiredDatasetCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		svc.MarkDatasetPurged, svc.GuardDatasetObjectRemoval, 100)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func expireDataset(t *testing.T, pool *pgxpool.Pool, datasetID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE datasets SET expires_at = now() - interval '1 hour' WHERE id = $1",
		mustUUID(t, datasetID)); err != nil {
		t.Fatal(err)
	}
}

func TestTheDatasetRetentionSweepTakesTheExpiredFileAndOnlyThatOne(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "dataset-retention")

	_, testCaseID := newTestCase(t, pool, a, c, "retention")
	oldID, oldKey := seedDataset(t, pool, a, c, testCaseID)
	freshID, freshKey := seedDataset(t, pool, a, c, testCaseID)
	expireDataset(t, pool, oldID)
	if _, err := pool.Exec(context.Background(), `INSERT INTO object_reconcile_sightings
		(resource_kind, resource_id, object_key, rounds) VALUES ('dataset', $1, $2, 2)`,
		mustUUID(t, oldID), oldKey); err != nil {
		t.Fatal(err)
	}

	if n := purgeDatasets(t, pool, a.packages); n != 1 {
		t.Fatalf("datasets purged: got %d, want 1", n)
	}

	if _, ok := a.packages[oldKey]; ok {
		t.Error("the expired dataset's file is still in the object store; the 90 days were kept on paper only")
	}
	if _, ok := a.packages[freshKey]; !ok {
		t.Error("a dataset still inside its retention window lost its file")
	}

	if n := countRows(t, pool,
		"SELECT count(*) FROM datasets WHERE id = $1 AND deleted_at IS NOT NULL",
		mustUUID(t, oldID)); n != 1 {
		t.Error("the expired row still claims a file that has been removed")
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM object_reconcile_sightings WHERE resource_kind = 'dataset' AND resource_id = $1",
		mustUUID(t, oldID)); n != 0 {
		t.Error("the expired dataset left a stale missing-object sighting")
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM datasets WHERE id = $1 AND deleted_at IS NULL",
		mustUUID(t, freshID)); n != 1 {
		t.Error("a dataset inside its window was marked deleted")
	}

	if n := purgeDatasets(t, pool, a.packages); n != 0 {
		t.Errorf("a second pass found %d rows; the first pass did not finish what it started", n)
	}
}
