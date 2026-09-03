// The retention sweep behind `maintenance purge-datasets` (PDM-006 §6, SEC-006,
// consent §3), through the real upload endpoint and the real object store.
//
// This file exists because the sweep did not. Migration 0004 created
// `datasets_expires_at_idx` and, in the comment directly above it, described the
// design of the sweep that would use it — "scans for expired rows rather than
// scheduling at creation time, so a shortened retention policy applies to
// already-stored data". The column shipped, the index shipped, the reasoning
// shipped, and nothing ever ran. Until 2026-08-25 the only statement that
// deleted a dataset was account deletion, so a participant who kept their
// account kept every file they had ever uploaded, permanently, against a 90-day
// number the upload screen had already quoted them and the consent form repeats.
//
// That is the third row of the same consent table to be caught the same way
// inside two days (audit events at 400 days, run outputs at 30, datasets at 90),
// which is why the assertions below are about the bytes and not about the
// return value: a sweep that reports a count and leaves the file in the store
// keeps the promise on paper only, and paper was the whole defect.
package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

// purgeDatasets runs the sweep the way the subcommand wires it: testlab owns
// both the worklist and the row write, because `datasets` is testlab's table and
// a generic scanner may not write another context's rows (ADR-033). Only the
// object-then-row ordering is shared with the two artifact halves.
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

// expireDataset backdates one row. The upload endpoint writes
// testlab.DatasetRetention, and a test that waited ninety days for it would not
// be a test.
func expireDataset(t *testing.T, pool *pgxpool.Pool, datasetID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE datasets SET expires_at = now() - interval '1 hour' WHERE id = $1",
		mustUUID(t, datasetID)); err != nil {
		t.Fatal(err)
	}
}

// Both directions in one test, deliberately. A test that only proved the expired
// file goes would stay green if the predicate were dropped entirely and the
// sweep took every dataset in the database — which is the one way this job can
// fail that costs somebody their work rather than merely failing to keep a
// promise.
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

	// The bytes, first, because this is what the consent form is about.
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

	// Iron rule 9: the sweep has to be safe to run twice, because a cron entry
	// that fires while the last one is still draining is the normal case, not the
	// exceptional one.
	if n := purgeDatasets(t, pool, a.packages); n != 0 {
		t.Errorf("a second pass found %d rows; the first pass did not finish what it started", n)
	}
}
