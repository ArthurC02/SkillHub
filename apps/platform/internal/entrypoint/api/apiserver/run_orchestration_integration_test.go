package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func withProvider(
	t *testing.T, a *api, pool *pgxpool.Pool, plan providertest.Plan, evaluator ...*eval.Service,
) (*providertest.Fake, *run.Service) {
	t.Helper()
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	fake.Plan = plan
	t.Cleanup(fake.Close)

	registry := run.NewRegistry(fake.Provider())

	a.runs.Providers = registry

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
	if final.FailureClass.Value != "" {
		t.Errorf("a successful run carries failure_class %q", final.FailureClass.Value)
	}

	if !strings.Contains(final.StatusReason, "另一個判斷") ||
		!strings.Contains(final.StatusReason, "評估") {
		t.Errorf("success reason = %q, want it to keep execution and task verdict apart "+
			"and to point at the evaluation", final.StatusReason)
	}
	if strings.Contains(final.StatusReason, "EVAL-001") {
		t.Errorf("the overturned TODO's wording is still here: %q", final.StatusReason)
	}

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

	waitForCleanup(t, f.client, created.RunID)
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes are still held after cleanup", fake.Live())
	}
}

func TestAFinishedRunIsEvaluatedThroughItsDomainEventExactlyOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-event-driven-eval")
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1})

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	deadline := time.Now().Add(20 * time.Second)
	for evaluations(t, pool, created.RunID) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := evaluations(t, pool, created.RunID); n != 1 {
		t.Fatalf("the run's domain event produced %d evaluations, want exactly 1", n)
	}

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

	if view.Status != string(gen.RunStatusRunning) {
		t.Errorf("status right after cancel = %q, want running", view.Status)
	}

	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusCancelled))
	if final.FailureClass.Value != "cancelled" {
		t.Errorf("failure_class = %q, want cancelled", final.FailureClass.Value)
	}
	waitForCleanup(t, f.client, created.RunID)
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes survived a cancelled run", fake.Live())
	}
}

func TestDispatchFailuresAreRetriedWithNewAttempts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-dispatch-retry")
	fake, _ := withProvider(t, a, pool, providertest.Plan{})

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
	if final.FailureClass.Value != "provider_error" {
		t.Errorf("failure_class = %q, want provider_error", final.FailureClass.Value)
	}
	if len(final.Attempts) != 2 {
		t.Errorf("attempts = %d, want the configured ceiling of 2", len(final.Attempts))
	}
}

func TestWorkloadFailureIsRecordedOnceAndNotRetried(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-workload-failure")
	fake, _ := withProvider(t, a, pool, providertest.Plan{
		FinalState: run.ProviderStateCompleted, ResultStatus: "failed", ErrorClass: "execution",
	})

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	if final.FailureClass.Value != "workload_error" {
		t.Errorf("failure_class = %q, want workload_error", final.FailureClass.Value)
	}
	if len(final.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1: a workload failure is not retried", len(final.Attempts))
	}
	if fake.Dispatches() != 1 {
		t.Errorf("the provider was dispatched to %d times after a workload failure", fake.Dispatches())
	}
	waitForCleanup(t, f.client, created.RunID)
}

func TestARunPastItsTokenCeilingIsStoppedByTheWorker(t *testing.T) {
	pool := requireDB(t)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/spend/logs") {

			_, _ = w.Write([]byte(`{"data":[{"prompt_tokens":420000,"completion_tokens":900}],"total_pages":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"sk-virtual-test"}`))
	}))
	defer gateway.Close()
	t.Setenv("SKILLHUB_MODEL_GATEWAY_URL", gateway.URL)
	t.Setenv("SKILLHUB_MODEL_GATEWAY_KEY", "sk-master-test")

	a := newAPI(t, pool)
	a.runs.Gateway = run.GatewayFromEnv()
	f := newFixture(t, a, pool, "alice-token-ceiling")
	fake, _ := withProvider(t, a, pool, providertest.Plan{StuckRunning: true})

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))

	if final.FailureClass.Value != "workload_error" {
		t.Errorf("failure_class = %q, want workload_error", final.FailureClass.Value)
	}

	if !strings.Contains(final.StatusReason, "token ceiling") {
		t.Errorf("reason = %q, want it to name the token ceiling", final.StatusReason)
	}

	if cls := attemptErrorClass(t, pool, created.RunID); cls != "budget_exhausted" {
		t.Errorf("error_class = %q, want budget_exhausted", cls)
	}

	waitForCleanup(t, f.client, created.RunID)
	if fake.Live() != 0 {
		t.Errorf("%d sandboxes survived a run stopped at its token ceiling", fake.Live())
	}
}

func TestSupervisorTimesOutARunThatOutlivedItsWallClock(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-wall-clock")
	created := f.start(t)

	ctx := context.Background()

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
	if view.FailureClass.Value != "timeout" {
		t.Errorf("failure_class = %q, want timeout", view.FailureClass.Value)
	}

	if !strings.Contains(view.StatusReason, "時間上限") {
		t.Errorf("reason = %q, want it to name the wall clock limit", view.StatusReason)
	}
}

func TestSupervisorRecoversARunThatHasNoJob(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-recovery")
	fake, _ := withProvider(t, a, pool, providertest.Plan{})

	orphanedSvc := *a.runs
	orphanedSvc.Providers = run.NewRegistry(fake.Provider())
	orphanedSvc.Store = a.packages
	orphanedSvc.Queue = nil
	ws, actor := mustUUID(t, f.workspaceID), mustUUID(t, f.userID)
	skill, version, testCase := mustUUID(t, f.skillID), mustUUID(t, f.versionID), mustUUID(t, f.testCaseID)

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

	if err := a.runs.Supervise(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, f.client, runID, string(gen.RunStatusSucceeded))
}

func TestARunWithNoAttemptToResumeIsTerminatedSafely(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-unresumable")
	ctx := context.Background()

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
	abandoned, err := gen.New(pool).CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
		ID: runID, WorkspaceID: ws, Provider: "sandbox",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Drive(ctx, ws, runID); err != nil {
		t.Fatal(err)
	}

	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusFailed) {
		t.Fatalf("status = %q, want failed", view.Status)
	}
	if view.FailureClass.Value != "platform_error" {
		t.Errorf("failure_class = %q, want platform_error", view.FailureClass.Value)
	}
	if !strings.Contains(view.StatusReason, "resume") {
		t.Errorf("reason = %q, want it to say the attempt could not be resumed", view.StatusReason)
	}
	var finite bool
	if err := pool.QueryRow(ctx, `SELECT object_grants_expire_at <> 'infinity'::timestamptz
		FROM run_attempts WHERE id = $1`, abandoned.ID).Scan(&finite); err != nil {
		t.Fatal(err)
	}
	if !finite {
		t.Fatal("abandoned pre-dispatch attempt kept an infinite account-purge fence")
	}
}

func TestLegacyAttemptGrantStateRemainsFailClosed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-legacy-grants")
	ctx := context.Background()
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
	attempt, err := gen.New(pool).CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
		ID: runID, WorkspaceID: ws, Provider: "sandbox",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `UPDATE run_attempts
		SET object_grants_state = 'legacy_unknown', object_grants_expire_at = 'infinity'
		WHERE id = $1`, attempt.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Drive(ctx, ws, runID); err != nil {
		t.Fatal(err)
	}
	var state string
	var stillInfinite bool
	if err := pool.QueryRow(ctx, `SELECT object_grants_state,
		object_grants_expire_at = 'infinity'::timestamptz
		FROM run_attempts WHERE id = $1`, attempt.ID).Scan(&state, &stillInfinite); err != nil {
		t.Fatal(err)
	}
	if state != "legacy_unknown" || !stillInfinite {
		t.Fatalf("legacy grant marker became state=%q infinity=%v; it must remain fail-closed", state, stillInfinite)
	}
}

func TestARunInterruptedBetweenEvaluatingAndSucceededResumes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()

	for _, tc := range []struct {
		name        string
		errorClass  *string
		wantStatus  gen.RunStatus
		wantFailure string
	}{
		{name: "alice-resume-evaluating", errorClass: nil,
			wantStatus: gen.RunStatusSucceeded, wantFailure: ""},

		{name: "alice-resume-refused", errorClass: strptr("execution_error"),
			wantStatus: gen.RunStatusFailed, wantFailure: "platform_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, a, pool, tc.name)

			svc := &run.Service{Pool: pool}
			created := f.start(t)
			ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)

			if _, err := pool.Exec(ctx, `
				INSERT INTO run_attempts (run_id, workspace_id, attempt_number, provider,
				                          provider_run_id, started_at, finished_at, error_class)
				VALUES ($1, $2, 1, 'fake_sandbox', 'sandbox-'||gen_random_uuid()::text, now(), now(), $3)`,
				runID, ws, tc.errorClass); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct{ from, to gen.RunStatus }{
				{gen.RunStatusQueued, gen.RunStatusProvisioning},
				{gen.RunStatusProvisioning, gen.RunStatusPreparing},
				{gen.RunStatusPreparing, gen.RunStatusRunning},
				{gen.RunStatusRunning, gen.RunStatusEvaluating},
			} {
				if _, err := svc.Transition(ctx, run.TransitionParams{
					WorkspaceID: ws, RunID: runID, From: step.from, To: step.to, Reason: "by hand",
				}); err != nil {
					t.Fatal(err)
				}
			}

			if err := svc.Drive(ctx, ws, runID); err != nil {
				t.Fatal(err)
			}

			_, view := f.getRun(t, created.RunID)
			if view.Status != string(tc.wantStatus) {
				t.Fatalf("status = %q (%s), want %s", view.Status, view.StatusReason, tc.wantStatus)
			}
			if view.FailureClass.Value != tc.wantFailure {
				t.Errorf("failure_class = %q, want %q", view.FailureClass.Value, tc.wantFailure)
			}
		})
	}
}

func strptr(s string) *string { return &s }

func TestARefusedTeardownIsRecordedAsFailedAndCleaningUpAgainIsSafe(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-cleanup-retry")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})

	fake.DestroyStatus = http.StatusInternalServerError

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	if got := waitForCleanupOutcome(t, f.client, created.RunID); got != "failed" {
		t.Fatalf("cleanup_status = %q after the provider refused the teardown, want failed", got)
	}
	before := fake.Destroys()

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

func TestOrphanScanDestroysLeakedSandboxesButSparesFreshOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-orphan-scan")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})

	leaked := fake.Seed("00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002", time.Now().Add(-time.Hour))

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

func TestARedispatchDoesNotRewriteTheRuntimeItAlreadyPinned(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-runtime-pin")
	_, svc := idleProvider(t, a, pool)
	ctx := context.Background()

	created := f.start(t)
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)

	const pinned = `{"provider":"fake_sandbox","runtime":{"image_digest":"sha256:the-one-attempt-1-matched"}}`
	if _, err := pool.Exec(ctx,
		`UPDATE runs SET provider = 'fake_sandbox', runtime_snapshot = $2 WHERE id = $1`,
		runID, pinned); err != nil {
		t.Fatal(err)
	}

	if err := svc.Drive(ctx, ws, runID); err != nil {
		t.Fatal(err)
	}

	var got string
	if err := pool.QueryRow(ctx,
		"SELECT runtime_snapshot::text FROM runs WHERE id = $1", runID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "the-one-attempt-1-matched") {
		t.Fatalf("the re-dispatch overwrote the runtime the first attempt matched: %s", got)
	}
}

func idleProvider(t *testing.T, a *api, pool *pgxpool.Pool) (*providertest.Fake, *run.Service) {
	t.Helper()
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	registry := run.NewRegistry(fake.Provider())
	a.runs.Providers = registry
	svc := *a.runs
	svc.Providers = registry
	svc.Store = a.packages
	svc.PollInterval = 20 * time.Millisecond
	return fake, &svc
}

func handlelessAttempt(t *testing.T, pool *pgxpool.Pool, runID, ws pgtype.UUID, number int) string {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO run_attempts (run_id, workspace_id, attempt_number, provider, started_at)
		VALUES ($1, $2, $3, 'fake_sandbox', now())
		RETURNING id`, runID, ws, number).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return uuidText(id)
}

func TestOrphanScanReclaimsASandboxWhoseHandleWasNeverRecorded(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-orphan-unrecorded")
	fake, svc := idleProvider(t, a, pool)
	ctx := context.Background()

	created := f.start(t)
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	lost := handlelessAttempt(t, pool, runID, ws, 1)
	inFlight := handlelessAttempt(t, pool, runID, ws, 2)

	leaked := fake.Seed(created.RunID, lost, time.Now().Add(-time.Hour))

	fresh := fake.Seed(created.RunID, inFlight, time.Now())

	if err := (&run.OrphanScanWorker{Svc: svc}).Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.Provider().GetRun(ctx, leaked); err == nil {
		t.Error("a sandbox whose attempt never recorded its handle survived the scan")
	}
	if _, err := fake.Provider().GetRun(ctx, fresh); err != nil {
		t.Errorf("the scan destroyed a dispatch that is still in flight: %v", err)
	}
}

func TestOrphanSightingsCountConsecutiveRoundsNotTotalFailures(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	newFixture(t, a, pool, "alice-orphan-rounds")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})
	ctx := context.Background()

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

	fake.Seed("00000000-0000-4000-8000-000000000013",
		"00000000-0000-4000-8000-000000000014", time.Now().Add(-time.Hour))
	scan()
	if got := persistentOrphans(t, pool, fake.Name); got != 1 {
		t.Errorf("a second, freshly-seen leak raised the count to %d, want 1", got)
	}

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

func TestOutboxPublisherIsAtLeastOnceAndIdempotent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-publisher")
	f.start(t)

	ctx := context.Background()
	if unpublishedCount(t, pool) == 0 {
		t.Fatal("creating a run published nothing to the outbox")
	}

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

	if n, err := publisher.Publish(ctx); err != nil || n != 0 {
		t.Errorf("second pass published %d events (err %v), want 0", n, err)
	}
}

// The claim lock is held only while a batch is claimed, not during delivery,
// so a second publisher can run concurrently and may redeliver an event the
// first has already claimed but not yet marked.
func TestConcurrentOutboxPublishersAreAtLeastOnceAndNeverLoseAnEvent(t *testing.T) {
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
	var firstDeliveries int64
	first := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		atomic.AddInt64(&firstDeliveries, 1)
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

	var secondDeliveries int64
	second := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		atomic.AddInt64(&secondDeliveries, 1)
		return nil
	}}
	if _, err := second.Publish(context.Background()); err != nil {
		t.Fatalf("a second publisher failed while the first was delivering: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&firstDeliveries) + atomic.LoadInt64(&secondDeliveries); got < int64(backlog) {
		t.Errorf("%d deliveries for a backlog of %d; at-least-once means at least once", got, backlog)
	}
	if after := unpublishedCount(t, pool); after != 0 {
		t.Fatalf("%d of %d events are still unpublished after both publishers finished", after, backlog)
	}

	var third int64
	trailing := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error {
		atomic.AddInt64(&third, 1)
		return nil
	}}
	if n, err := trailing.Publish(context.Background()); err != nil || n != 0 || third != 0 {
		t.Errorf("a pass after the backlog drained published %d and delivered %d (err %v), want 0 and 0", n, third, err)
	}
}

func TestIncompatibleWorkIsRefusedBeforeItIsQueued(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-incompatible")

	fake := providertest.New("weak_sandbox", "test-token")
	t.Cleanup(fake.Close)
	weak := providertest.DefaultCapability("weak_sandbox")
	weak.MaxResources.MemoryBytes = 1 << 28
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

func (c *client) runPage(t *testing.T, query string) []runListView {
	t.Helper()
	var out struct {
		Runs []runListView `json:"runs"`
	}
	url := c.base + "/runs" + query
	if code := getJSON(t, c.Client, url, &out); code != http.StatusOK {
		t.Fatalf("GET %s: got %d", url, code)
	}
	return out.Runs
}

func TestRunHistoryRefusesOutOfSchemaPaging(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, uniqueWorklistLabel("alice-run-paging"))

	f.start(t)
	f.start(t)

	for _, query := range []string{
		"limit=0", "limit=201", "limit=abc", "limit=", "limit=-1", "limit=1.5",
		"offset=-1", "offset=abc", "offset=", "offset=1.5", "offset=2147483648",

		"test_case_id=not-a-uuid&limit=0",
		"test_case_id=not-a-uuid&offset=-1",
	} {
		if code, body := f.doJSON(t, http.MethodGet, "/runs?"+query, ""); code != http.StatusBadRequest {
			t.Errorf("GET /runs?%s: got %d, want 400 (body %v)", query, code, body)
		}
	}

	for _, query := range []string{
		"", "?limit=1", "?limit=200", "?offset=0", "?offset=2147483647",
		"?limit=200&offset=0",
	} {
		if code, body := f.doJSON(t, http.MethodGet, "/runs"+query, ""); code != http.StatusOK {
			t.Errorf("GET /runs%s: got %d, want 200 (body %v)", query, code, body)
		}
	}

	if rows := f.runPage(t, ""); len(rows) != 2 {
		t.Fatalf("unfiltered history = %d runs, want 2", len(rows))
	}
	capped := f.runPage(t, "?limit=1")
	if len(capped) != 1 {
		t.Fatalf("limit=1 returned %d runs, want 1", len(capped))
	}
	skipped := f.runPage(t, "?offset=1")
	if len(skipped) != 1 {
		t.Fatalf("offset=1 returned %d runs, want 1", len(skipped))
	}

	if skipped[0].RunID == capped[0].RunID {
		t.Errorf("offset=1 led with the same run limit=1 did (%s), so it was ignored", capped[0].RunID)
	}
	if rows := f.runPage(t, "?offset=2"); len(rows) != 0 {
		t.Errorf("offset past the end returned %d runs, want 0", len(rows))
	}
}

func attemptErrorClass(t *testing.T, pool *pgxpool.Pool, runID string) string {
	t.Helper()
	var class *string
	if err := pool.QueryRow(context.Background(), `
		SELECT error_class FROM run_attempts
		WHERE run_id = $1 ORDER BY attempt_number DESC LIMIT 1`,
		mustUUID(t, runID)).Scan(&class); err != nil {
		t.Fatal(err)
	}
	if class == nil {
		return ""
	}
	return *class
}
