// Run scheduling and resilience, end to end against a fake sandbox provider
// (RUN-005~008 plus the outbox publisher). Same file family as
// run_integration_test.go, and the same rule: the real route table, the real
// River worker, the real state machine, a throwaway database.
//
// The provider is a fake (internal/run/providertest) that implements the frozen
// contract. It isolates nothing and runs nothing — these tests are about the
// orchestrator, not about the sandbox (ADR-015, SEC-009 cover that).
package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

// withProvider starts a fake provider, points both the API and a worker at it,
// and returns the fake plus the worker's service. Poll and retry settings are
// squeezed so a test finishes in milliseconds rather than minutes.
func withProvider(
	t *testing.T, a *api, pool *pgxpool.Pool, plan providertest.Plan, evaluator ...*eval.Service,
) (*providertest.Fake, *run.Service) {
	t.Helper()
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	fake.Plan = plan
	t.Cleanup(fake.Close)

	registry := run.NewRegistry(fake.Provider())
	// The API refuses incompatible work before queueing, so it needs the registry
	// too (RUN-005); it never dispatches.
	a.runs.Providers = registry

	// Store, as cmd/worker wires it: a dispatch mints the object grants a sandbox
	// fetches its inputs with, and a worker without one refuses to dispatch at all
	// (SBX-008 is fail-closed).
	svc := *a.runs
	svc.Providers = registry
	svc.Store = a.packages
	svc.PollInterval = 20 * time.Millisecond
	evalSvc := a.evaluations
	if len(evaluator) == 1 {
		evalSvc = evaluator[0]
	}
	startWorkerWith(t, &svc, evalSvc)
	return fake, &svc
}

// clearRunBacklog is fixture hygiene, not product behaviour.
//
// Several tests in this package deliberately create runs with nobody to work them
// — that is what they are testing. They share one database and one River queue
// with the tests below, so a worker started here would pick every one of those
// jobs up and dispatch it to this test's fake provider: slow, and it moves
// counters this test is about to assert on.
//
// The rows are retired with a direct UPDATE rather than through the state machine.
// There is no "abandon" transition and inventing one so a test could tidy up would
// be a worse trade than one honest UPDATE in a test helper.
func clearRunBacklog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM river_job`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE runs
		SET status = 'failed', finished_at = now(), cleanup_status = 'cleaned',
		    status_reason = 'abandoned by a later test'
		WHERE status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')`); err != nil {
		t.Fatal(err)
	}
}

func waitForCleanup(t *testing.T, c *client, runID string) runView {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last runView
	for time.Now().Before(deadline) {
		_, last = c.getRun(t, runID)
		if last.CleanupStatus.Value == string(gen.RunCleanupStatusCleaned) {
			return last
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("run %s never finished cleanup; last cleanup_status %+v", runID, last.CleanupStatus)
	return last
}

// SBX-008's dataset half: a run whose test case carries an uploaded file has to
// be dispatched with a read grant for that file's *current* object key.
//
// The key is not in the snapshot — a DatasetRef freezes the content hash, because
// that is what outlives the file, while the key is a storage fact (ADR-003 刪除與
// 可追溯性). So the dispatcher re-reads the row, through testlab.ReadDataset since
// DDD-033. Nothing exercised that branch of grantsFor before: every other fixture
// runs a test case with no files, so a dispatcher that could not resolve a dataset
// at all still made every run in this file go green.
//
// The assertion is that the run finishes. Grant minting is fail-closed and runs
// before anything reaches a sandbox, so a lookup that cannot find the dataset
// fails the dispatch — "it succeeded" is exactly the statement that the file was
// resolved.
func TestARunWhoseTestCaseCarriesAFileIsDispatchedWithAGrantForIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-dataset-run")
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO datasets (workspace_id, test_case_id, file_name, content_type,
		                      size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'input.csv', 'text/csv', 9, 'sha256:dispatch-input',
		        'datasets/dispatch-input.csv', now() + interval '90 days')`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID)); err != nil {
		t.Fatal(err)
	}
	fake, _ := withProvider(t, a, pool, providertest.Plan{RunningPolls: 1})

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))
	if final.StatusReason == "" {
		t.Error("a terminal transition records no reason")
	}
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d, want 1", fake.Dispatches())
	}
}

// RUN-005 + RUN-007, the happy path: a provider is selected, the run walks the
// whole state machine, and the sandbox is released afterwards.
func TestRunWalksTheStateMachineAndIsCleanedUp(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-happy-run")
	fake, _ := withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1})

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	var path []string
	for _, tr := range final.Transitions {
		path = append(path, tr.To)
		if tr.Reason == "" {
			t.Errorf("transition to %s recorded no reason", tr.To)
		}
	}
	want := []string{"queued", "provisioning", "preparing", "running", "evaluating", "succeeded"}
	if strings.Join(path, ",") != strings.Join(want, ",") {
		t.Errorf("transition path = %v, want %v", path, want)
	}
	if final.FailureClass != "" {
		t.Errorf("a successful run carries failure_class %q", final.FailureClass)
	}
	// ADR-025: the terminal reason is execution language and nothing more. It used
	// to promise an evaluator that would come back and decide `succeeded` versus
	// `failed`; that TODO was overturned, and `succeeded` must not read as a task
	// verdict on any surface.
	if !strings.Contains(final.StatusReason, "separate judgement") {
		t.Errorf("success reason = %q, want it to keep execution and task verdict apart", final.StatusReason)
	}
	if strings.Contains(final.StatusReason, "EVAL-001") {
		t.Errorf("the overturned TODO's wording is still here: %q", final.StatusReason)
	}

	// RUN-005: the chosen provider is on the run, and the mapping is on the attempt.
	if final.Provider != "fake_sandbox" {
		t.Errorf("run provider = %q, want fake_sandbox", final.Provider)
	}
	if len(final.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(final.Attempts))
	}
	if final.Attempts[0].ProviderRunID == "" {
		t.Error("the attempt recorded no provider_run_id, so the sandbox could never be reconciled")
	}
	if fake.Dispatches() != 1 {
		t.Errorf("the provider was dispatched to %d times for one run", fake.Dispatches())
	}

	// RUN-007: cleanup follows the terminal state without anyone asking.
	waitForCleanup(t, f.client, created.RunID)
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes are still held after cleanup", fake.Live())
	}
}

// DDD-005 / contracts/events/domain-events.md §4 rule 5: nothing in internal/run
// enqueues an evaluation any more. The whole chain has to work for a finished run
// to get one — terminal transition writes `run.succeeded`, the publisher hands it
// to eval's consumer, the consumer enqueues — and it has to produce exactly one,
// however many times the at-least-once outbox delivers the event.
func TestAFinishedRunIsEvaluatedThroughItsDomainEventExactlyOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-event-driven-eval")
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1})

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	// The consumer runs off the periodic publish startWorkerWith registers, so the
	// evaluation appears on its own — no test calls Evaluate here.
	deadline := time.Now().Add(20 * time.Second)
	for evaluations(t, pool, created.RunID) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := evaluations(t, pool, created.RunID); n != 1 {
		t.Fatalf("the run's domain event produced %d evaluations, want exactly 1", n)
	}

	// Redelivery: the run's own committed event, handed to the consumer a second
	// time the way a publisher that died before marking it published would. A
	// second evaluation here is a second paid judge call for one run.
	//
	// Delivered directly rather than by re-running the publisher: the backlog is
	// shared with every other test in this package, and this assertion is about
	// one run's event.
	var event outbox.Event
	if err := pool.QueryRow(context.Background(), `
		SELECT event_id, event_type, event_version, occurred_at, correlation_id,
		       causation_id, workspace_id, aggregate_type, aggregate_id, payload, published_at
		FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'run.succeeded'`,
		mustUUID(t, created.RunID),
	).Scan(&event.EventID, &event.EventType, &event.EventVersion, &event.OccurredAt,
		&event.CorrelationID, &event.CausationID, &event.WorkspaceID, &event.AggregateType,
		&event.AggregateID, &event.Payload, &event.PublishedAt); err != nil {
		t.Fatal(err)
	}
	consumer := &eval.RunEventConsumer{
		HasCurrentEvaluation: a.evaluations.HasCurrentEvaluation,
		Insert: func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
			t.Error("a redelivered run.succeeded enqueued a second evaluation")
			return nil, nil
		},
	}
	if err := consumer.Deliver(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if n := evaluations(t, pool, created.RunID); n != 1 {
		t.Fatalf("after redelivery the run has %d evaluations, want 1", n)
	}
}

func evaluations(t *testing.T, pool *pgxpool.Pool, runID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM evaluations WHERE run_id = $1", mustUUID(t, runID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// RUN-006: cancelling a dispatched run reaches the provider, and the run only
// reports `cancelled` once the workload is actually down.
func TestCancelReachesTheProviderAndStopsTheRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-cancel-live")
	fake, _ := withProvider(t, a, pool, providertest.Plan{StuckRunning: true})

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusRunning))

	code, view := f.postJSON(t, "/runs/"+created.RunID+"/cancel", "")
	if code != http.StatusAccepted {
		t.Fatalf("cancel: got %d, want 202", code)
	}
	// Intent only, while the workload is still up.
	if view.Status != string(gen.RunStatusRunning) {
		t.Errorf("status right after cancel = %q, want running", view.Status)
	}

	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusCancelled))
	if final.FailureClass != "cancelled" {
		t.Errorf("failure_class = %q, want cancelled", final.FailureClass)
	}
	waitForCleanup(t, f.client, created.RunID)
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes survived a cancelled run", fake.Live())
	}
}

// RUN-006: a provider that cannot take the dispatch is retried, with a new attempt
// row each time so the previous provider mapping is never overwritten (RUN-003).
func TestDispatchFailuresAreRetriedWithNewAttempts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-dispatch-retry")
	fake, _ := withProvider(t, a, pool, providertest.Plan{})
	// Two bad minutes, then the provider recovers.
	fake.DispatchStatuses = []int{http.StatusServiceUnavailable, http.StatusTooManyRequests}

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	if len(final.Attempts) != 3 {
		t.Fatalf("attempts = %d, want 3 (two refused dispatches and one that took)", len(final.Attempts))
	}
	for i, attempt := range final.Attempts[:2] {
		if attempt.ErrorClass != "provision" {
			t.Errorf("attempt %d error_class = %q, want provision", i+1, attempt.ErrorClass)
		}
		if attempt.ProviderRunID != "" {
			t.Errorf("attempt %d recorded a provider handle for a dispatch that was refused", i+1)
		}
	}
	if final.Attempts[2].ProviderRunID == "" {
		t.Error("the attempt that succeeded recorded no provider handle")
	}
}

// ADR-004: no unbounded retries. A provider that never recovers ends the run after
// the configured ceiling, classified as the provider's failure and not the skill's.
func TestRetriesAreBoundedAndClassifiedAsProviderFailure(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-retry-ceiling")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})
	svc.MaxAttempts = 2
	fake.DispatchStatuses = []int{
		http.StatusServiceUnavailable, http.StatusServiceUnavailable,
		http.StatusServiceUnavailable, http.StatusServiceUnavailable,
	}

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	if final.FailureClass != "provider_error" {
		t.Errorf("failure_class = %q, want provider_error", final.FailureClass)
	}
	if len(final.Attempts) != 2 {
		t.Errorf("attempts = %d, want the configured ceiling of 2", len(final.Attempts))
	}
}

// RUN-006's other half: the workload ran and reported failure. That is the skill's
// answer, not a transient fault, so it is recorded once and never retried.
func TestWorkloadFailureIsRecordedOnceAndNotRetried(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-workload-failure")
	fake, _ := withProvider(t, a, pool, providertest.Plan{
		FinalState: run.ProviderStateCompleted, ResultStatus: "failed", ErrorClass: "execution",
	})

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	if final.FailureClass != "workload_error" {
		t.Errorf("failure_class = %q, want workload_error", final.FailureClass)
	}
	if len(final.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1: a workload failure is not retried", len(final.Attempts))
	}
	if fake.Dispatches() != 1 {
		t.Errorf("the provider was dispatched to %d times after a workload failure", fake.Dispatches())
	}
	waitForCleanup(t, f.client, created.RunID)
}

// RUN-006 wall clock, RUN-008 watchdog: a run whose deadline passed while nothing
// was driving it is timed out by the supervisor rather than left running forever.
func TestSupervisorTimesOutARunThatOutlivedItsWallClock(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-wall-clock")
	created := f.start(t)

	ctx := context.Background()
	// Squeeze the run's own frozen wall clock, then push it into the past. Both
	// writes are legal because the run is not terminal (0005).
	if _, err := pool.Exec(ctx, `
		UPDATE runs
		SET policy_snapshot = jsonb_set(policy_snapshot,
		        '{resource_limits,wall_clock_hard_seconds}', '1'),
		    created_at = now() - interval '1 hour'
		WHERE id = $1`, mustUUID(t, created.RunID)); err != nil {
		t.Fatal(err)
	}

	svc := &run.Service{Pool: pool}
	if err := svc.Supervise(ctx); err != nil {
		t.Fatal(err)
	}

	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusTimedOut) {
		t.Fatalf("status = %q, want timed_out", view.Status)
	}
	if view.FailureClass != "timeout" {
		t.Errorf("failure_class = %q, want timeout", view.FailureClass)
	}
	if !strings.Contains(view.StatusReason, "wall clock") {
		t.Errorf("reason = %q, want it to name the wall clock", view.StatusReason)
	}
}

// RUN-008 recovery: a run that has no job at all — the process died between the
// run row and the queue, or the job was lost — is picked up again by the
// supervisor and driven to completion.
func TestSupervisorRecoversARunThatHasNoJob(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-recovery")
	fake, _ := withProvider(t, a, pool, providertest.Plan{})

	// A service with no queue creates the run and enqueues nothing, which is
	// exactly the state a crash between the two would leave behind.
	orphanedSvc := *a.runs
	orphanedSvc.Providers = run.NewRegistry(fake.Provider())
	orphanedSvc.Store = a.packages
	orphanedSvc.Queue = nil
	ws, actor := mustUUID(t, f.workspaceID), mustUUID(t, f.userID)
	skill, version, testCase := mustUUID(t, f.skillID), mustUUID(t, f.versionID), mustUUID(t, f.testCaseID)
	// Gate B applies to every caller of Create, this one included: confirm through
	// the same service, because the summary names the provider it would dispatch to
	// and this service has a different fleet from the API's.
	summary, err := orphanedSvc.PermissionSummaryFor(context.Background(), ws, skill, version, testCase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orphanedSvc.ConfirmPermissions(context.Background(), ws, actor, skill, version, testCase, summary.Hash); err != nil {
		t.Fatal(err)
	}
	created, err := orphanedSvc.Create(context.Background(), run.CreateParams{
		WorkspaceID: ws, Actor: actor,
		SkillID:              skill,
		VersionID:            version,
		TestCaseID:           testCase,
		ConfirmedSummaryHash: summary.Hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := uuidText(created.ID)
	if _, view := f.getRun(t, runID); view.Status != string(gen.RunStatusQueued) {
		t.Fatalf("the un-queued run is %q, want queued", view.Status)
	}

	// The worker's own service has a queue; one sweep gives the run a job.
	if err := a.runs.Supervise(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, f.client, runID, string(gen.RunStatusSucceeded))
}

// RUN-008 safe termination: a run past dispatch with no attempt to resume cannot
// be rewound — the state machine has no backward edge — so it is failed honestly
// rather than silently re-run as if it were the original attempt.
func TestARunWithNoAttemptToResumeIsTerminatedSafely(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-unresumable")
	ctx := context.Background()

	// No worker here: this test drives the job itself, so the run stays exactly
	// where it is put.
	created := f.start(t)
	svc := &run.Service{Pool: pool}
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	for _, step := range []struct{ from, to gen.RunStatus }{
		{gen.RunStatusQueued, gen.RunStatusProvisioning},
		{gen.RunStatusProvisioning, gen.RunStatusPreparing},
	} {
		if _, err := svc.Transition(ctx, run.TransitionParams{
			WorkspaceID: ws, RunID: runID, From: step.from, To: step.to, Reason: "by hand",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A restart lands here: the run is past dispatch and no attempt was ever
	// recorded, so there is nothing to re-attach to.
	if err := svc.Drive(ctx, ws, runID); err != nil {
		t.Fatal(err)
	}

	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusFailed) {
		t.Fatalf("status = %q, want failed", view.Status)
	}
	if view.FailureClass != "platform_error" {
		t.Errorf("failure_class = %q, want platform_error", view.FailureClass)
	}
	if !strings.Contains(view.StatusReason, "resume") {
		t.Errorf("reason = %q, want it to say the attempt could not be resumed", view.StatusReason)
	}
}

// RUN-007: a teardown the provider refused is recorded as failed, and running
// the whole thing again afterwards is safe.
//
// Both halves were unasserted. `cleanup_status = 'failed'` appears nowhere in
// any test in this repository, so turning that write into `'cleaned'` was green
// — and O11Y-003's alert rules exist precisely to tell the two apart: a
// `destroyed` is a leak that was contained, a `failed` is a leak still burning a
// slot and the only one that needs a human. The early return on an
// already-cleaned run had no test either, so deleting it was green as well,
// which is the one thing iron rule 9 asks of this path (M2 audit, 2026-08-24).
func TestARefusedTeardownIsRecordedAsFailedAndCleaningUpAgainIsSafe(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-cleanup-retry")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})

	// The provider will not let go of this sandbox.
	fake.DestroyStatus = http.StatusInternalServerError

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	if got := waitForCleanupOutcome(t, f.client, created.RunID); got != "failed" {
		t.Fatalf("cleanup_status = %q after the provider refused the teardown, want failed", got)
	}
	before := fake.Destroys()

	// The provider recovers. Cleanup runs again and this time succeeds.
	fake.DestroyStatus = 0
	runRow := readRun(t, pool, f.workspaceID, created.RunID)
	if err := svc.Cleanup(context.Background(), runRow); err != nil {
		t.Fatalf("retrying a failed cleanup: %v", err)
	}
	if got := runCleanupStatus(t, pool, created.RunID); got != string(gen.RunCleanupStatusCleaned) {
		t.Fatalf("cleanup_status = %q after a successful retry, want cleaned", got)
	}
	if after := fake.Destroys(); after <= before {
		t.Fatalf("the retry never reached the provider: %d destroys before, %d after", before, after)
	}

	// And a third cleanup job over an already-cleaned run does nothing at all.
	// Driven through CleanupWorker rather than Cleanup, because that is where the
	// early return lives: Cleanup itself is safe to repeat by contract (DELETE has
	// no 404), and the worker is what stops a redelivered job from paying for it.
	settled := fake.Destroys()
	job := &river.Job[run.CleanupArgs]{
		Args: run.CleanupArgs{RunID: created.RunID, WorkspaceID: f.workspaceID},
	}
	if err := (&run.CleanupWorker{Svc: svc}).Work(context.Background(), job); err != nil {
		t.Fatalf("a cleanup job for an already-cleaned run: %v", err)
	}
	if got := fake.Destroys(); got != settled {
		t.Errorf("a cleaned run was torn down again: %d destroys, want %d", got, settled)
	}
}

// readRun is the row Cleanup takes. Workspace-scoped like every other read of
// it (iron rule 3), which is why the fixture's workspace comes along.
func readRun(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string) gen.Run {
	t.Helper()
	var id, ws pgtype.UUID
	if err := id.Scan(runID); err != nil {
		t.Fatal(err)
	}
	if err := ws.Scan(workspaceID); err != nil {
		t.Fatal(err)
	}
	row, err := gen.New(pool).GetRun(context.Background(), gen.GetRunParams{ID: id, WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func runCleanupStatus(t *testing.T, pool *pgxpool.Pool, runID string) string {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(runID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(context.Background(),
		"SELECT cleanup_status::text FROM runs WHERE id = $1", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// RUN-007 orphan scanning: a sandbox the platform has no live attempt for is
// destroyed, and a fresh one it does not recognise yet is left alone.
func TestOrphanScanDestroysLeakedSandboxesButSparesFreshOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-orphan-scan")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})

	// A sandbox from a run this platform never knew about, old enough to be past
	// the dispatch window.
	leaked := fake.Seed("00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002", time.Now().Add(-time.Hour))
	// And one created a moment ago: unrecognised, but possibly a dispatch still in
	// flight, so it must survive.
	fresh := fake.Seed("00000000-0000-4000-8000-000000000003",
		"00000000-0000-4000-8000-000000000004", time.Now())

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))
	waitForCleanup(t, f.client, created.RunID)

	if err := (&run.OrphanScanWorker{Svc: svc}).Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if _, err := fake.Provider().GetRun(context.Background(), leaked); err == nil {
		t.Error("the leaked sandbox survived the scan")
	}
	if _, err := fake.Provider().GetRun(context.Background(), fresh); err != nil {
		t.Errorf("the scan destroyed a sandbox that was too new to judge: %v", err)
	}
}

// SBX-012 / ADR-022 X-03: the in-flight orphan table, which is what makes
// "同一筆連續 2 輪仍存在" a thing the platform can actually say.
//
// The distinction the old accumulating counter could not draw, and this can: two
// different resources each failing once is not the same event as one resource
// stuck for two rounds, and only the second is the alert.
func TestOrphanSightingsCountConsecutiveRoundsNotTotalFailures(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	newFixture(t, a, pool, "alice-orphan-rounds")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})
	ctx := context.Background()

	// A leak the provider refuses to destroy: it is still there next round.
	fake.DestroyStatus = http.StatusInternalServerError
	stuck := fake.Seed("00000000-0000-4000-8000-000000000011",
		"00000000-0000-4000-8000-000000000012", time.Now().Add(-time.Hour))

	scan := func() { _ = (&run.OrphanScanWorker{Svc: svc}).Work(ctx, nil) }

	scan()
	if got := persistentOrphans(t, pool, fake.Name); got != 0 {
		t.Fatalf("after one round the count is %d, want 0 — one sighting is not two", got)
	}
	scan()
	if got := persistentOrphans(t, pool, fake.Name); got != 1 {
		t.Fatalf("after two consecutive rounds on the same handle the count is %d, want 1", got)
	}
	if _, err := fake.Provider().GetRun(ctx, stuck); err != nil {
		t.Fatalf("the fixture stopped holding the stuck sandbox: %v", err)
	}

	// A *different* handle failing once must not add to it: the threshold is about
	// one resource surviving, not about how much failed in the window.
	fake.Seed("00000000-0000-4000-8000-000000000013",
		"00000000-0000-4000-8000-000000000014", time.Now().Add(-time.Hour))
	scan()
	if got := persistentOrphans(t, pool, fake.Name); got != 1 {
		t.Errorf("a second, freshly-seen leak raised the count to %d, want 1", got)
	}

	// Once the provider can tear it down again, the next round destroys everything
	// and the round after that finds nothing to carry — the count clears itself.
	fake.DestroyStatus = 0
	scan()
	scan()
	if got := persistentOrphans(t, pool, fake.Name); got != 0 {
		t.Errorf("count after the leaks were cleared = %d, want 0", got)
	}
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes still held after the scan could destroy again", fake.Live())
	}
}

func persistentOrphans(t *testing.T, pool *pgxpool.Pool, provider string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM reconciler_orphan_sightings WHERE provider = $1 AND rounds >= 2",
		provider).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ADR-008 / iron rule 9: events are handed on at least once, marking is idempotent,
// and a delivery that fails leaves the backlog where it was.
func TestOutboxPublisherIsAtLeastOnceAndIdempotent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-publisher")
	f.start(t)

	ctx := context.Background()
	if unpublishedCount(t, pool) == 0 {
		t.Fatal("creating a run published nothing to the outbox")
	}

	// A delivery that fails must not mark anything: at-least-once means the event
	// is still there to be re-sent. It must also be *reported* — the pass returns
	// the failure so River retries the publish job, instead of the backlog sitting
	// there until the next tick with nobody the wiser.
	backlog := unpublishedCount(t, pool)
	failing := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		return context.DeadlineExceeded
	}}
	n, err := failing.Publish(ctx)
	if n != 0 {
		t.Fatalf("failed delivery published %d events, want 0", n)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failed delivery returned %v, want the delivery error", err)
	}
	if after := unpublishedCount(t, pool); after != backlog {
		t.Errorf("a failed delivery changed the backlog from %d to %d", backlog, after)
	}

	var delivered []string
	publisher := &outbox.Worker{Pool: pool, Deliver: func(_ context.Context, e outbox.Event) error {
		delivered = append(delivered, e.EventType)
		return nil
	}}
	n, err = publisher.Publish(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != backlog || len(delivered) != backlog {
		t.Errorf("published %d events and delivered %d, want %d", n, len(delivered), backlog)
	}
	if after := unpublishedCount(t, pool); after != 0 {
		t.Errorf("%d events are still unpublished after a successful pass", after)
	}
	// Draining again is a no-op rather than a re-delivery storm.
	if n, err := publisher.Publish(ctx); err != nil || n != 0 {
		t.Errorf("second pass published %d events (err %v), want 0", n, err)
	}
}

func TestConcurrentOutboxPublishersDoNotDeliverTheSameSnapshot(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-concurrent")
	f.start(t)
	backlog := unpublishedCount(t, pool)
	if backlog == 0 {
		t.Fatal("creating a run published nothing to the outbox")
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	result := make(chan error, 1)
	first := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}}
	go func() {
		_, err := first.Publish(context.Background())
		result <- err
	}()
	<-started

	secondDeliveries := 0
	second := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		secondDeliveries++
		return nil
	}}
	if n, err := second.Publish(context.Background()); err != nil || n != 0 {
		t.Fatalf("concurrent publisher published %d events (err %v), want 0", n, err)
	}
	if secondDeliveries != 0 {
		t.Fatalf("concurrent publisher delivered %d events, want 0", secondDeliveries)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if after := unpublishedCount(t, pool); after != 0 {
		t.Fatalf("first publisher left %d of %d events unpublished", after, backlog)
	}
}

// RUN-005 / ADR-004: work no configured provider can carry is refused before it is
// queued, with a reason the user can read — not queued and quietly failed later.
func TestIncompatibleWorkIsRefusedBeforeItIsQueued(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-incompatible")

	fake := providertest.New("weak_sandbox", "test-token")
	t.Cleanup(fake.Close)
	weak := providertest.DefaultCapability("weak_sandbox")
	weak.MaxResources.MemoryBytes = 1 << 28 // 256 MiB: nowhere near the run's 4 GiB
	fake.Capability = &weak
	a.runs.Providers = run.NewRegistry(fake.Provider())

	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+`"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on an incompatible fleet: got %d, want 422", code)
	}
	if !strings.Contains(view.Error, "memory") || !strings.Contains(view.Error, "weak_sandbox") {
		t.Errorf("refusal = %q, want it to name the provider and what did not fit", view.Error)
	}
	if fake.Dispatches() != 0 {
		t.Error("a refused run was still dispatched")
	}
}
