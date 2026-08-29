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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestRegistryTransactionalReadFaceKeepsScopeAndUncommittedVisibility(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "registry-read-owner")
	stranger := a.login(t, "registry-read-stranger")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ownerID := mustUUID(t, owner.workspaceID)
	strangerID := mustUUID(t, stranger.workspaceID)
	var skillID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO skills (workspace_id, name, summary)
		VALUES ($1, 'transaction-visible', 'inside tx') RETURNING id`, ownerID).Scan(&skillID); err != nil {
		t.Fatal(err)
	}
	byName, found, err := registry.SkillByName(ctx, tx, ownerID, "transaction-visible")
	if err != nil || !found || byName.ID != skillID {
		t.Fatalf("owner read of uncommitted Skill = %+v, found=%v, err=%v", byName, found, err)
	}
	if _, found, err := registry.SkillByName(ctx, tx, strangerID, "transaction-visible"); err != nil || found {
		t.Fatalf("foreign workspace found owner Skill: found=%v, err=%v", found, err)
	}
	if byID, found, err := registry.SkillByID(ctx, tx, ownerID, skillID); err != nil || !found || byID.Name != "transaction-visible" {
		t.Fatalf("owner read by id = %+v, found=%v, err=%v", byID, found, err)
	}
	if _, found, err := registry.SkillByID(ctx, tx, strangerID, skillID); err != nil || found {
		t.Fatalf("foreign workspace found owner Skill by id: found=%v, err=%v", found, err)
	}

	var versionID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO skill_versions
			(workspace_id, skill_id, version_number, content_hash, package_object_key)
		VALUES ($1, $2, 1, 'sha256:transaction-visible', 'packages/transaction-visible')
		RETURNING id`, ownerID, skillID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	version, found, err := registry.VersionByContent(ctx, tx, ownerID, skillID, "sha256:transaction-visible")
	if err != nil || !found || version.ID != versionID || version.VersionNumber != 1 {
		t.Fatalf("duplicate read of uncommitted version = %+v, found=%v, err=%v", version, found, err)
	}
}

// TestTestLabReadFaceIsWorkspaceScoped covers the four functions internal/trial/improvement,
// internal/run and internal/packaging now go through, plus the object-key lister
// internal/creator/workspace's account purge is handed.
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
	testlabSvc := &testlab.Service{Pool: pool}

	ws := mustUUID(t, owner.workspaceID)
	foreign := mustUUID(t, stranger.workspaceID)
	skillID := seedSkill(t, pool, owner.workspaceID, "read-face-skill")
	testCaseID := mustUUID(t, seedTestCase(t, pool, owner.workspaceID, skillID))

	const datasetKey = "datasets/read-face.csv"
	const artifactKey = "artifacts/read-face.zip"
	const runArtifactKey = "run-artifacts/x/y/artifacts.tar"
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := testlabSvc.CreateSnapshot(ctx, tx, ws, testCaseID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	versionID := seedSkillVersion(t, pool, owner.workspaceID, skillID)
	var runID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider)
		VALUES ($1, $2, $3, 'read-face') RETURNING id`,
		ws, mustUUID(t, versionID), snapshot.ID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (workspace_id, run_id, kind, file_name, content_type,
		                       size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'run_output', 'result.zip', 'application/zip', 10,
		        'sha256:read-face-run', $3, now() + interval '30 days')`,
		ws, runID, runArtifactKey); err != nil {
		t.Fatal(err)
	}

	t.Run("ReadSnapshot", func(t *testing.T) {
		got, err := testlabSvc.ReadSnapshot(ctx, ws, snapshot.ID)
		if err != nil {
			t.Fatalf("owner cannot read the snapshot its own run points at: %v", err)
		}
		if got.TestCaseID != testCaseID {
			t.Errorf("snapshot names test case %v, want %v", got.TestCaseID, testCaseID)
		}
		if len(got.AcceptanceCriteria) == 0 {
			t.Error("the frozen criteria came back empty; eval judges against these")
		}
		if _, err := testlabSvc.ReadSnapshot(ctx, foreign, snapshot.ID); !errors.Is(err, testlab.ErrNotFound) {
			t.Errorf("another workspace reading the snapshot got %v, want ErrNotFound", err)
		}
	})

	t.Run("ReadDataset", func(t *testing.T) {
		got, err := testlabSvc.ReadDataset(ctx, ws, datasetID)
		if err != nil {
			t.Fatalf("owner cannot read its own dataset: %v", err)
		}
		if got.ObjectKey != datasetKey {
			t.Errorf("object_key = %q, want %q; run mints the read grant from this", got.ObjectKey, datasetKey)
		}
		if _, err := testlabSvc.ReadDataset(ctx, foreign, datasetID); !errors.Is(err, testlab.ErrNotFound) {
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
		if _, err := testlabSvc.ReadDataset(ctx, ws, datasetID); !errors.Is(err, testlab.ErrNotFound) {
			t.Errorf("a deleted dataset read as %v, want ErrNotFound", err)
		}
	})

	t.Run("CasesForSkill", func(t *testing.T) {
		rows, err := testlabSvc.CasesForSkill(ctx, ws, mustUUID(t, skillID))
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != testCaseID {
			t.Fatalf("owner sees %d cases for its own skill, want exactly the seeded one", len(rows))
		}
		// Empty and not an error: packaging asks this about a skill it is already
		// entitled to package, so "none of yours" is the answer, not a refusal.
		other, err := testlabSvc.CasesForSkill(ctx, foreign, mustUUID(t, skillID))
		if err != nil {
			t.Fatal(err)
		}
		if len(other) != 0 {
			t.Errorf("another workspace sees %d of the owner's test cases, want 0", len(other))
		}
	})

	t.Run("CaseDatasets", func(t *testing.T) {
		rows, err := testlabSvc.CaseDatasets(ctx, ws, testCaseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ObjectKey != datasetKey {
			t.Fatalf("owner sees %d files on its own case, want the seeded one", len(rows))
		}
		other, err := testlabSvc.CaseDatasets(ctx, foreign, testCaseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(other) != 0 {
			t.Errorf("another workspace sees %d of the owner's files, want 0", len(other))
		}
	})

	t.Run("WorkspaceObjectKeys", func(t *testing.T) {
		testlabSvc := &testlab.Service{Pool: pool}
		runSvc := &run.Service{Pool: pool}
		packagingSvc := &packaging.Service{Pool: pool}
		for _, tc := range []struct {
			name string
			list identity.WorkspaceObjectKeys
			want string
		}{
			{"testlab", testlabSvc.WorkspaceObjectKeys, datasetKey},
			{"run", runSvc.WorkspaceObjectKeys, runArtifactKey},
			{"packaging", packagingSvc.WorkspaceObjectKeys, artifactKey},
		} {
			t.Run(tc.name, func(t *testing.T) {
				keys, err := tc.list(ctx, ws)
				if err != nil {
					t.Fatal(err)
				}
				if len(keys) != 1 || keys[0] != tc.want {
					t.Fatalf("object keys = %v, want only %q", keys, tc.want)
				}
				strangerKeys, err := tc.list(ctx, foreign)
				if err != nil {
					t.Fatal(err)
				}
				if len(strangerKeys) != 0 {
					t.Fatalf("another workspace sees object keys %v", strangerKeys)
				}
			})
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
