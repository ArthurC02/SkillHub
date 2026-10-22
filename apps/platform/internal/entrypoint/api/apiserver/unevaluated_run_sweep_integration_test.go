package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type finishedRunSeed struct {
	pool               *pgxpool.Pool
	workspace, version string
	snapshot           string
	now                time.Time
}

func (s finishedRunSeed) run(t *testing.T, status string, finishedAgo time.Duration) string {
	t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, status, provider, finished_at)
		VALUES ($1, $2, $3, $4, 'test', $5) RETURNING id::text`,
		mustUUID(t, s.workspace), mustUUID(t, s.version), mustUUID(t, s.snapshot), status, s.now.Add(-finishedAgo)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAFinishedRunWhoseEvaluationNeverStartedIsEnqueuedAgain(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	tag := uniqueWorklistLabel("unevaluated-run")
	f := newFixture(t, a, pool, tag)
	seed := finishedRunSeed{pool: pool, workspace: f.workspaceID, version: f.versionID, now: time.Now()}
	if err := pool.QueryRow(ctx, `INSERT INTO test_case_snapshots
		(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'sweep', '[]', $3) RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID), tag).Scan(&seed.snapshot); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		seed.run(t, "succeeded", time.Hour):      true,
		seed.run(t, "failed", 23*time.Hour):      true,
		seed.run(t, "cancelled", time.Hour):      false,
		seed.run(t, "succeeded", 10*time.Minute): false,
		seed.run(t, "succeeded", 25*time.Hour):   false,
	}
	evaluated := seed.run(t, "succeeded", time.Hour)
	want[evaluated] = false
	if _, err := pool.Exec(ctx, `INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete)
		VALUES ($1, $2, 'pending', 'undetermined', false)`, mustUUID(t, f.workspaceID), mustUUID(t, evaluated)); err != nil {
		t.Fatal(err)
	}

	consumer := wiring.NewRunEventConsumer(pool, &eval.Service{Pool: pool}, &run.Service{Pool: pool})
	enqueued := map[string]bool{}
	consumer.Enqueue = func(_ context.Context, args eval.JobArgs) error {
		enqueued[args.RunID] = true
		return nil
	}
	if _, err := consumer.EnqueueUnevaluated(ctx, seed.now); err != nil {
		t.Fatal(err)
	}
	for id, shouldEnqueue := range want {
		if enqueued[id] != shouldEnqueue {
			t.Errorf("run %s enqueued = %v, want %v", id, enqueued[id], shouldEnqueue)
		}
	}
}
