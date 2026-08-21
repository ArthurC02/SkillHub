// The owner-side read functions DDD-033 added, so that eval, run, packaging and
// identity could stop calling testlab's and trace's queries themselves
// (ADR-035 C 組 and G 組).
//
// "The call site came back" is deliberately NOT tested here: `devctl
// automation-check` fails the build on any non-owner call to those seven
// queries, and a Go test asserting the same thing would be a second, weaker copy
// of a check that already runs in CI. What automation-check cannot see is a
// wrapper that compiles, is called from the right place, and answers the wrong
// thing — which for a read face means, above everything else, losing the
// workspace scope. A read that dropped it would hand one workspace another's
// test data with every call site looking exactly as it does now (iron rule 3,
// WS-006).
//
// These live in apiserver rather than in testlab and trace because neither of
// those packages has a database test harness, and giving them one would make a
// fourth and a fifth package resetting the shared `public` schema.
//
// Not here: trace.MaskingActivity. It is deployment-wide rather than workspace
// scoped, and it already has a behavioural test that discriminates its arguments
// — TestMaskingStoppedHaltsDispatchWithoutAnOperator drives all four branches of
// the two-window rule, and swapping `recent` for `since` makes earlier_events
// always zero, so the halt it asserts never happens.
package apiserver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
)

// TestTestLabReadFaceIsWorkspaceScoped covers the four functions internal/eval,
// internal/run and internal/packaging now go through, plus the object-key lister
// internal/identity's account purge is handed.
//
// Every case is the same shape twice: the owning workspace gets the row, and a
// second real workspace gets "there is no such thing" — never someone else's
// row, and never a different error that would let a caller tell the difference.
func TestTestLabReadFaceIsWorkspaceScoped(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "read-face-owner")
	stranger := a.login(t, "read-face-stranger")
	ctx := context.Background()
	q := gen.New(pool)

	ws := mustUUID(t, owner.workspaceID)
	foreign := mustUUID(t, stranger.workspaceID)
	skillID := seedSkill(t, pool, owner.workspaceID, "read-face-skill")
	testCaseID := mustUUID(t, seedTestCase(t, pool, owner.workspaceID, skillID))

	const datasetKey = "datasets/read-face.csv"
	const artifactKey = "artifacts/read-face.zip"
	var datasetID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO datasets (workspace_id, test_case_id, file_name, content_type,
		                      size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'input.csv', 'text/csv', 10, 'sha256:read-face', $3,
		        now() + interval '90 days')
		RETURNING id`,
		ws, testCaseID, datasetKey).Scan(&datasetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (workspace_id, kind, file_name, content_type,
		                       size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, 'download_package', 'pkg.zip', 'application/zip', 10,
		        'sha256:read-face-pkg', $2, now() + interval '30 days')`,
		ws, artifactKey); err != nil {
		t.Fatal(err)
	}

	// The owner's own write path, so the snapshot under test is the one a run
	// would actually execute rather than a hand-built row.
	snapshot, err := testlab.CreateSnapshot(ctx, q, ws, testCaseID)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	t.Run("ReadSnapshot", func(t *testing.T) {
		got, err := testlab.ReadSnapshot(ctx, q, ws, snapshot.ID)
		if err != nil {
			t.Fatalf("owner cannot read the snapshot its own run points at: %v", err)
		}
		if got.TestCaseID != testCaseID {
			t.Errorf("snapshot names test case %v, want %v", got.TestCaseID, testCaseID)
		}
		if len(got.AcceptanceCriteria) == 0 {
			t.Error("the frozen criteria came back empty; eval judges against these")
		}
		if _, err := testlab.ReadSnapshot(ctx, q, foreign, snapshot.ID); !errors.Is(err, testlab.ErrNotFound) {
			t.Errorf("another workspace reading the snapshot got %v, want ErrNotFound", err)
		}
	})

	t.Run("ReadDataset", func(t *testing.T) {
		got, err := testlab.ReadDataset(ctx, q, ws, datasetID)
		if err != nil {
			t.Fatalf("owner cannot read its own dataset: %v", err)
		}
		if got.ObjectKey != datasetKey {
			t.Errorf("object_key = %q, want %q; run mints the read grant from this", got.ObjectKey, datasetKey)
		}
		if _, err := testlab.ReadDataset(ctx, q, foreign, datasetID); !errors.Is(err, testlab.ErrNotFound) {
			t.Errorf("another workspace reading the dataset got %v, want ErrNotFound", err)
		}
		// A deleted file has to read as gone, or a run whose input the user
		// removed would be dispatched with a grant for bytes that are not there.
		if _, err := pool.Exec(ctx,
			"UPDATE datasets SET deleted_at = now() WHERE id = $1", datasetID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := pool.Exec(context.Background(),
				"UPDATE datasets SET deleted_at = NULL WHERE id = $1", datasetID); err != nil {
				t.Fatal(err)
			}
		})
		if _, err := testlab.ReadDataset(ctx, q, ws, datasetID); !errors.Is(err, testlab.ErrNotFound) {
			t.Errorf("a deleted dataset read as %v, want ErrNotFound", err)
		}
	})

	t.Run("CasesForSkill", func(t *testing.T) {
		rows, err := testlab.CasesForSkill(ctx, q, ws, mustUUID(t, skillID))
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != testCaseID {
			t.Fatalf("owner sees %d cases for its own skill, want exactly the seeded one", len(rows))
		}
		// Empty and not an error: packaging asks this about a skill it is already
		// entitled to package, so "none of yours" is the answer, not a refusal.
		other, err := testlab.CasesForSkill(ctx, q, foreign, mustUUID(t, skillID))
		if err != nil {
			t.Fatal(err)
		}
		if len(other) != 0 {
			t.Errorf("another workspace sees %d of the owner's test cases, want 0", len(other))
		}
	})

	t.Run("CaseDatasets", func(t *testing.T) {
		rows, err := testlab.CaseDatasets(ctx, q, ws, testCaseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ObjectKey != datasetKey {
			t.Fatalf("owner sees %d files on its own case, want the seeded one", len(rows))
		}
		other, err := testlab.CaseDatasets(ctx, q, foreign, testCaseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(other) != 0 {
			t.Errorf("another workspace sees %d of the owner's files, want 0", len(other))
		}
	})

	t.Run("WorkspaceObjectKeys", func(t *testing.T) {
		keys, err := testlab.WorkspaceObjectKeys(ctx, q, ws)
		if err != nil {
			t.Fatal(err)
		}
		// Both halves of the union, by name. The account purge deletes exactly
		// what this returns, so a half that stopped being listed is a user's file
		// left alive after they were told it was gone (CORE-007) — and a count
		// would not say which half.
		got := map[string]bool{}
		for _, k := range keys {
			got[k] = true
		}
		if !got[datasetKey] {
			t.Errorf("the dataset file is not listed for deletion: %v", keys)
		}
		if !got[artifactKey] {
			t.Errorf("the artifact file is not listed for deletion: %v", keys)
		}
		strangerKeys, err := testlab.WorkspaceObjectKeys(ctx, q, foreign)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range strangerKeys {
			if k == datasetKey || k == artifactKey {
				t.Fatalf("purging one workspace would delete another's file %q", k)
			}
		}
	})
}

// TestTraceLiveEventsIsScopedToOneRunInOneWorkspace covers what eval's report
// asks of trace on every read: of the events this report cites, which still
// exist (ADR-026 decision 2)?
//
// The scope is the whole answer. Widened to the workspace, one run's report would
// keep claiming a citation resolves because a different run happens to have an
// event with that id; widened past the workspace it would do so across accounts.
func TestTraceLiveEventsIsScopedToOneRunInOneWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "live-events-owner")
	stranger := a.login(t, "live-events-stranger")
	ctx := context.Background()

	ws := mustUUID(t, owner.workspaceID)
	skillID := seedSkill(t, pool, owner.workspaceID, "live-events")
	runID, _ := seedEvaluatableRun(t, pool, owner.workspaceID, skillID)
	// A second skill, because seedEvaluatableRun derives the version's content
	// hash from the skill and two versions cannot share one.
	otherRunID, _ := seedEvaluatableRun(t, pool, owner.workspaceID,
		seedSkill(t, pool, owner.workspaceID, "live-events-other"))

	cited := seedToolCallEvent(t, pool, owner.workspaceID, runID, 41, "read-face-cited")
	elsewhere := seedToolCallEvent(t, pool, owner.workspaceID, otherRunID, 42, "read-face-elsewhere")
	var absent pgtype.UUID
	if err := absent.Scan("00000000-0000-4000-8000-0000000000ff"); err != nil {
		t.Fatal(err)
	}

	live, err := a.evaluations.Trace.LiveEvents(ctx, ws, mustUUID(t, runID),
		[]pgtype.UUID{cited, elsewhere, absent})
	if err != nil {
		t.Fatal(err)
	}
	got := map[pgtype.UUID]bool{}
	for _, u := range live {
		got[u] = true
	}
	if !got[cited] {
		t.Error("the event this run really carries came back as gone; every citation of it would be labelled stale")
	}
	if got[elsewhere] {
		t.Error("an event belonging to another run came back live; a report would claim a citation resolves when it does not")
	}
	if got[absent] {
		t.Error("an id that is in no row at all came back live")
	}

	crossWorkspace, err := a.evaluations.Trace.LiveEvents(ctx,
		mustUUID(t, stranger.workspaceID), mustUUID(t, runID), []pgtype.UUID{cited})
	if err != nil {
		t.Fatal(err)
	}
	if len(crossWorkspace) != 0 {
		t.Errorf("another workspace resolved %d of the owner's events, want 0", len(crossWorkspace))
	}
}

// seedToolCallEvent writes one masked tool_call event straight to the table and
// returns its id. Direct insert rather than the ingestion endpoint because these
// tests are about who may read an event, not about how one arrives.
func seedToolCallEvent(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string, seq int, tool string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
		                          event_type, source, status, schema_version, masked,
		                          masked_fields, payload)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, now(), 'tool_call', 'agent_sdk', 'ok',
		        '1.0', true, '[]'::jsonb,
		        jsonb_build_object('tool_name', $4::text, 'arguments', '{}'::jsonb))
		RETURNING event_id`,
		mustUUID(t, workspaceID), mustUUID(t, runID), seq, tool,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
