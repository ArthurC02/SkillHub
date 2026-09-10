package apiserver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func seedTestCase(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt, acceptance_criteria)
		VALUES ($1, $2, 'run test', 'summarise the attached csv', '[{"id":"c1","text":"produces a summary"}]'::jsonb)
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, skillID),
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type runView struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
	SkillID      string `json:"skill_id"`
	TestCaseID   string `json:"test_case_id"`
	Provider     string `json:"provider"`

	FailureClass  labelledJSON `json:"failure_class"`
	CleanupStatus labelledJSON `json:"cleanup_status"`
	Error         string       `json:"error"`
	Transitions   []struct {
		From   string `json:"from_status"`
		To     string `json:"to_status"`
		Reason string `json:"reason"`
	} `json:"transitions"`
	Attempts []struct {
		RunAttemptID  string `json:"run_attempt_id"`
		AttemptNumber int32  `json:"attempt_number"`
		ProviderRunID string `json:"provider_run_id"`
		ErrorClass    string `json:"error_class"`
	} `json:"attempts"`
	CancelRequestedAt string `json:"cancel_requested_at"`
}

func (c *client) postJSON(t *testing.T, path, body string) (int, runView) {
	t.Helper()
	resp, err := c.Post(c.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out runView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) getRun(t *testing.T, runID string) (int, runView) {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out runView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func startWorker(t *testing.T, a *api) {
	t.Helper()
	startWorkerWith(t, a.runs, a.evaluations)
}

func startWorkerWith(t *testing.T, svc *run.Service, evaluator *eval.Service) *river.Client[pgx.Tx] {
	t.Helper()
	if evaluator == nil {
		t.Fatal("worker evaluator is not wired")
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &run.Worker{Svc: svc})
	river.AddWorker(workers, &run.CleanupWorker{Svc: svc})
	river.AddWorker(workers, &run.OrphanScanWorker{Svc: svc})
	river.AddWorker(workers, &run.SuperviseWorker{Svc: svc})

	evalSvc := *evaluator
	river.AddWorker(workers, &eval.Worker{Svc: &evalSvc})

	runEvents := &eval.RunEventConsumer{HasCurrentEvaluation: evalSvc.HasCurrentEvaluation}
	outboxWorker := &outbox.Worker{
		Pool: svc.Pool, Deliver: runEvents.Deliver, PublishInterval: 200 * time.Millisecond,
	}
	river.AddWorker(workers, outboxWorker)
	c, err := queue.New(svc.Pool, &river.Config{
		Workers: workers,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(outboxWorker.Interval()),
				func() (river.JobArgs, *river.InsertOpts) { return outbox.PublishArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runEvents.Insert = c.Insert

	if svc.Queue == nil {
		svc.Queue = c
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	return c
}

func waitForStatus(t *testing.T, c *client, runID, want string) runView {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last runView
	for time.Now().Before(deadline) {
		code, view := c.getRun(t, runID)
		if code != http.StatusOK {
			t.Fatalf("GET /runs/%s: got %d", runID, code)
		}
		last = view
		if view.Status == want {
			return view
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %s; last status %q (%s)", runID, want, last.Status, last.StatusReason)
	return last
}

type fixture struct {
	*client
	skillID, versionID, testCaseID string
}

func newFixture(t *testing.T, a *api, pool *pgxpool.Pool, user string) fixture {
	t.Helper()
	c := a.login(t, user)
	skillID := seedSkill(t, pool, c.workspaceID, user+"-runnable-skill")
	version := seedVersion(t, pool, c.workspaceID, skillID, "hash-"+user)

	a.packages[version.PackageObjectKey] = cleanPackage(t)
	return fixture{
		client:     c,
		skillID:    skillID,
		versionID:  uuidText(version.ID),
		testCaseID: seedTestCase(t, pool, c.workspaceID, skillID),
	}
}

func cleanPackage(t *testing.T) []byte {
	t.Helper()
	return zipOf(t, map[string]string{
		"SKILL.md": "---\nname: clean-skill\ndescription: A skill with no script.\nlicense: MIT\n---\n\nJust prose.\n",
	})
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f fixture) start(t *testing.T) runView {
	t.Helper()
	hash := f.confirmPermissions(t)
	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST run: got %d (%s)", code, view.Error)
	}
	return view
}

func TestRunFailsImmediatelyWhenNoProviderIsConfigured(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-run-chain")

	created := f.start(t)
	if created.Status != string(gen.RunStatusQueued) {
		t.Fatalf("new run status = %q, want queued", created.Status)
	}

	if created.Provider != "unassigned" {
		t.Errorf("new run provider = %q, want unassigned", created.Provider)
	}

	if created.SkillID != f.skillID || created.TestCaseID != f.testCaseID {
		t.Errorf("created run linkage = (%s, %s), want (%s, %s)",
			created.SkillID, created.TestCaseID, f.skillID, f.testCaseID)
	}
	if _, read := f.getRun(t, created.RunID); read.SkillID != f.skillID ||
		read.TestCaseID != f.testCaseID {
		t.Errorf("read run linkage = (%s, %s), want (%s, %s)",
			read.SkillID, read.TestCaseID, f.skillID, f.testCaseID)
	}

	startWorker(t, a)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))

	if !strings.Contains(final.StatusReason, "no sandbox provider") {
		t.Errorf("failure reason = %q, want it to name the missing provider", final.StatusReason)
	}

	if final.FailureClass.Value != "capability_mismatch" {
		t.Errorf("failure_class = %q, want capability_mismatch", final.FailureClass.Value)
	}

	var path []string
	for _, tr := range final.Transitions {
		path = append(path, tr.To)
		if tr.Reason == "" {
			t.Errorf("transition to %s recorded no reason", tr.To)
		}
	}
	want := []string{"queued", "failed"}
	if strings.Join(path, ",") != strings.Join(want, ",") {
		t.Errorf("transition path = %v, want %v", path, want)
	}
	if from := final.Transitions[0].From; from != "" {
		t.Errorf("first transition came from %q, want the run's creation (empty)", from)
	}

	if len(final.Attempts) != 0 {
		t.Errorf("attempts = %d, want 0: nothing was ever dispatched", len(final.Attempts))
	}

	waitForCleanup(t, f.client, created.RunID)
	events := outboxFor(t, pool, created.RunID)
	var types []string
	for _, e := range events {
		types = append(types, e.EventType)
		if got := uuidText(e.CorrelationID); got != created.RunID {
			t.Errorf("event %s correlates on %q, want the platform run_id %q", e.EventType, got, created.RunID)
		}
	}

	wantTypes := []string{"run.queued", "run.failed", "run.cleanup_cleaned"}
	if strings.Join(types, ",") != strings.Join(wantTypes, ",") {
		t.Errorf("outbox event types = %v, want %v", types, wantTypes)
	}
}

func TestFailedWritesLeaveNoOutboxEvent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-atomicity")
	q := gen.New(pool)
	ctx := context.Background()

	before := unpublishedCount(t, pool)

	other := newFixture(t, a, pool, "bob-outbox-atomicity")
	code, _ := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+other.testCaseID+`"}`)
	if code != http.StatusNotFound {
		t.Fatalf("run with another workspace's test case: got %d, want 404", code)
	}
	if after := unpublishedCount(t, pool); after != before {
		t.Errorf("rejected run creation leaked %d outbox events", after-before)
	}

	created := f.start(t)
	svc := &run.Service{Pool: pool}
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	move := run.TransitionParams{
		WorkspaceID: ws, RunID: runID,
		From: gen.RunStatusQueued, To: gen.RunStatusProvisioning, Reason: "first",
	}
	if _, err := svc.Transition(ctx, move); err != nil {
		t.Fatal(err)
	}
	afterFirst := unpublishedCount(t, pool)

	move.Reason = "second"
	if _, err := svc.Transition(ctx, move); err == nil || !strings.Contains(err.Error(), "no longer in the expected status") {
		t.Fatalf("replayed transition: got %v, want a conflict", err)
	}
	if after := unpublishedCount(t, pool); after != afterFirst {
		t.Errorf("conflicting transition leaked %d outbox events", after-afterFirst)
	}
	history, err := q.ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{RunID: runID, WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}

	if len(history) != 2 {
		t.Errorf("status history has %d rows, want 2", len(history))
	}
}

func failOutboxCommitFor(t *testing.T, pool *pgxpool.Pool, aggregateID pgtype.UUID) {
	t.Helper()
	// A deferred constraint trigger fires at COMMIT rather than at the INSERT, so
	// every earlier statement in the transaction has already run when it aborts
	// the commit, scoped to one run's aggregate id.

	ctx := context.Background()
	id := uuidText(aggregateID)
	name := "test_fail_outbox_" + strings.ReplaceAll(id, "-", "")
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION `+name+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected: this transaction must not commit'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE CONSTRAINT TRIGGER `+name+`
		AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW WHEN (NEW.aggregate_id = '`+id+`'::uuid)
		EXECUTE FUNCTION `+name+`()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+name+` ON outbox_events`); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+name+`()`); err != nil {
			t.Error(err)
		}
	})
}

func TestATransitionThatFailsAfterItsAuditWriteLeavesNoAuditRow(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-audit-atomicity")
	ctx := context.Background()

	created := f.start(t)
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	failOutboxCommitFor(t, pool, runID)

	svc := &run.Service{Pool: pool}
	if _, err := svc.Transition(ctx, run.TransitionParams{
		WorkspaceID: ws, RunID: runID,
		From: gen.RunStatusQueued, To: gen.RunStatusProvisioning, Reason: "cannot commit",
	}); err == nil {
		t.Fatal("the transition reported success although its transaction could not commit")
	}

	if n := countRow(t, pool,
		"SELECT count(*) FROM audit_events WHERE action = 'run.transition' AND resource_id = $1",
		runID); n != 0 {
		t.Errorf("%d run.transition audit rows outlived the transaction that wrote them", n)
	}

	if _, view := f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("status = %q, want queued: a transition that could not commit was applied", view.Status)
	}
}

func TestAnUndeliverableEventIsIsolatedAndReleasesTheBacklog(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-poison")

	poisoned := f.start(t)
	behind := f.start(t)
	poisonedID := mustUUID(t, poisoned.RunID)

	var attempts int
	w := &outbox.Worker{
		Pool:                pool,
		MaxDeliveryAttempts: 2,
		Deliver: func(_ context.Context, e outbox.Event) error {
			if e.AggregateID == poisonedID {
				attempts++
				return errors.New("this consumer will never accept this event")
			}
			return nil
		},
	}

	for range 5 {
		if _, err := w.Publish(context.Background()); err == nil && attempts >= 2 {
			break
		}
	}

	poison := eventOfType(t, pool, poisoned.RunID, outbox.RunQueued)
	if !poison.DeadLetteredAt.Valid {
		t.Fatalf("after %d failed deliveries the event was not isolated", attempts)
	}
	if poison.DeliveryAttempts != 2 {
		t.Errorf("delivery_attempts = %d, want 2 (the configured ceiling)", poison.DeliveryAttempts)
	}

	if poison.PublishedAt.Valid {
		t.Error("an event that was never accepted is marked published")
	}

	before := attempts
	if _, err := w.Publish(context.Background()); err != nil {
		t.Fatalf("the pass after isolation still failed: %v", err)
	}
	if attempts != before {
		t.Errorf("the isolated event was delivered %d more times; isolation means it stops", attempts-before)
	}

	if released := eventOfType(t, pool, behind.RunID, outbox.RunQueued); !released.PublishedAt.Valid {
		t.Error("the event committed after the poison never published: the backlog is still blocked")
	}
}

func TestThePublisherPrunesDeliveredEventsButKeepsIsolatedOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-retention")
	ws := mustUUID(t, f.workspaceID)

	old := time.Now().Add(-8 * 24 * time.Hour)
	stale := insertAgedOutboxEvent(t, pool, ws, old, nil)
	recent := insertAgedOutboxEvent(t, pool, ws, time.Now(), nil)

	poisoned := insertAgedOutboxEvent(t, pool, ws, old, &old)

	w := &outbox.Worker{Pool: pool, Deliver: func(context.Context, outbox.Event) error { return nil }}
	if _, err := w.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}

	if outboxRowExists(t, pool, stale) {
		t.Error("an event delivered 8 days ago is still in the buffer: retention did not run")
	}
	if !outboxRowExists(t, pool, recent) {
		t.Error("an event delivered just now was pruned: the retention window is not being honoured")
	}
	if !outboxRowExists(t, pool, poisoned) {
		t.Error("an isolated event was pruned; it is kept for a human, not for the publisher")
	}
}

func TestIllegalTransitionIsRefusedWithoutWriting(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-illegal-transition")
	created := f.start(t)

	svc := &run.Service{Pool: pool}
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	before := unpublishedCount(t, pool)

	_, err := svc.Transition(context.Background(), run.TransitionParams{
		WorkspaceID: ws, RunID: runID,
		From: gen.RunStatusQueued, To: gen.RunStatusSucceeded, Reason: "cheating",
	})
	if err == nil || !strings.Contains(err.Error(), "illegal run status transition") {
		t.Fatalf("queued -> succeeded: got %v, want an illegal-transition error", err)
	}
	if after := unpublishedCount(t, pool); after != before {
		t.Error("a refused transition still wrote to the outbox")
	}
	if _, view := f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("run status after refused transition = %q, want queued", view.Status)
	}
}

func TestRetryAddsAttemptWithoutOverwritingTheProviderMapping(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-attempt-mapping")
	created := f.start(t)

	ctx := context.Background()
	q := gen.New(pool)
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)

	var ids []string
	for i, providerRunID := range []string{"provider-sandbox-1", "provider-sandbox-2"} {
		attempt, err := q.CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
			ID: runID, WorkspaceID: ws, Provider: "fake",
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := int32(i + 1); attempt.AttemptNumber != want {
			t.Errorf("attempt number = %d, want %d", attempt.AttemptNumber, want)
		}
		if _, err := q.SetAttemptProviderRunID(ctx, gen.SetAttemptProviderRunIDParams{
			ID: attempt.ID, WorkspaceID: ws, ProviderRunID: &providerRunID,
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, uuidText(attempt.ID))
	}
	if ids[0] == ids[1] {
		t.Fatal("the retry reused the first attempt's run_attempt_id")
	}

	attempts, err := q.ListRunAttempts(ctx, gen.ListRunAttemptsParams{RunID: runID, WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}

	if attempts[0].ProviderRunID == nil || *attempts[0].ProviderRunID != "provider-sandbox-1" {
		t.Errorf("attempt 1 provider_run_id = %v, want provider-sandbox-1", attempts[0].ProviderRunID)
	}
	if attempts[1].ProviderRunID == nil || *attempts[1].ProviderRunID != "provider-sandbox-2" {
		t.Errorf("attempt 2 provider_run_id = %v, want provider-sandbox-2", attempts[1].ProviderRunID)
	}
}

func TestCancelRecordsIntentAndStopsAQueuedRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-cancel")
	created := f.start(t)

	code, view := f.postJSON(t, "/runs/"+created.RunID+"/cancel", "")
	if code != http.StatusAccepted {
		t.Fatalf("cancel: got %d, want 202", code)
	}
	if view.CancelRequestedAt == "" {
		t.Error("cancel did not record when it was requested")
	}

	if view.Status != string(gen.RunStatusQueued) {
		t.Errorf("status right after cancel = %q, want queued (the workload is not down yet)", view.Status)
	}

	if code, _ := f.postJSON(t, "/runs/"+created.RunID+"/cancel", ""); code != http.StatusAccepted {
		t.Errorf("second cancel: got %d, want 202", code)
	}

	startWorker(t, a)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusCancelled))
	if len(final.Attempts) != 0 {
		t.Errorf("a run cancelled before dispatch created %d attempts, want 0", len(final.Attempts))
	}

	if code, _ := f.postJSON(t, "/runs/"+created.RunID+"/cancel", ""); code != http.StatusConflict {
		t.Errorf("cancel of a finished run: got %d, want 409", code)
	}
}

func TestArtifactListReportsACompletelyDroppedCollection(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "artifact-collection-truncated")
	created := f.start(t)
	if _, err := pool.Exec(context.Background(),
		"UPDATE runs SET artifacts_truncated = true WHERE id = $1", mustUUID(t, created.RunID)); err != nil {
		t.Fatal(err)
	}

	resp, err := f.Get(f.base + "/runs/" + created.RunID + "/artifacts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Artifacts []json.RawMessage `json:"artifacts"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || len(out.Artifacts) != 0 || !out.Truncated {
		t.Fatalf("artifact list status=%d body=%+v; want empty artifacts with truncated=true", resp.StatusCode, out)
	}
}

func TestConcurrentArtifactManifestRedeliveryDoesNotDuplicateRows(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "artifact-manifest-redelivery")
	runID := mustUUID(t, f.start(t).RunID)
	workspaceID := mustUUID(t, f.workspaceID)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	params := gen.InsertRunArtifactParams{
		WorkspaceID: workspaceID, RunID: runID, FileName: "Report.txt",
		ContentType: "text/plain", SizeBytes: 1, ContentHash: "hash", ObjectKey: "runs/redelivery/archive",
	}

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx) //nolint:errcheck // no-op after commit
	q1 := gen.New(tx1)
	if err := q1.LockRunArtifactManifest(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if rows, err := q1.InsertRunArtifact(ctx, params); err != nil || rows != 1 {
		t.Fatalf("first manifest insert rows=%d err=%v", rows, err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		tx2, err := pool.Begin(ctx)
		if err != nil {
			done <- err
			return
		}
		defer tx2.Rollback(ctx) //nolint:errcheck // no-op after commit
		q2 := gen.New(tx2)
		close(started)
		if err := q2.LockRunArtifactManifest(ctx, runID); err != nil {
			done <- err
			return
		}
		second := params
		second.FileName = "report.txt"
		rows, err := q2.InsertRunArtifact(ctx, second)
		if err == nil && rows != 0 {
			err = fmt.Errorf("redelivery inserted %d duplicate rows", rows)
		}
		if err == nil {
			err = tx2.Commit(ctx)
		}
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("second insert completed before the first transaction released its manifest lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM artifacts
		WHERE run_id = $1 AND kind = 'run_output' AND lower(file_name) = 'report.txt'`, runID); n != 1 {
		t.Fatalf("portable manifest rows = %d, want 1", n)
	}
}

func TestRunsAreInvisibleAcrossWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := newFixture(t, a, pool, "alice-run-isolation")
	bob := newFixture(t, a, pool, "bob-run-isolation")
	created := alice.start(t)

	if code, _ := bob.getRun(t, created.RunID); code != http.StatusNotFound {
		t.Errorf("GET another workspace's run: got %d, want 404", code)
	}
	if code, _ := bob.postJSON(t, "/runs/"+created.RunID+"/cancel", ""); code != http.StatusNotFound {
		t.Errorf("cancel another workspace's run: got %d, want 404", code)
	}

	if code, _ := bob.postJSON(t, "/skills/"+alice.skillID+"/runs",
		`{"version_id":"`+bob.versionID+`","test_case_id":"`+bob.testCaseID+`"}`); code != http.StatusNotFound {
		t.Errorf("run on another workspace's skill: got %d, want 404", code)
	}

	if code, _ := bob.postJSON(t, "/skills/"+bob.skillID+"/runs",
		`{"version_id":"`+alice.versionID+`","test_case_id":"`+bob.testCaseID+`"}`); code != http.StatusNotFound {
		t.Errorf("run with another workspace's version: got %d, want 404", code)
	}

	if _, view := alice.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Error("a cross-workspace request changed the owner's run")
	}
}

func TestRunRejectsATestCaseBelongingToAnotherSkillInTheSameWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-run-testcase-scope")
	_, otherTestCaseID := newTestCase(t, pool, a, f.client, "other")

	path := "/skills/" + f.skillID + "/runs/preflight?version_id=" + f.versionID +
		"&test_case_id=" + otherTestCaseID
	code, _ := f.doJSON(t, http.MethodGet, path, "")
	if code != http.StatusNotFound {
		t.Fatalf("preflight paired a version with another skill's test case: got %d", code)
	}
}

func outboxFor(t *testing.T, pool *pgxpool.Pool, runID string) []gen.OutboxEvent {
	t.Helper()
	events, err := gen.New(pool).ListOutboxEventsByAggregate(context.Background(),
		gen.ListOutboxEventsByAggregateParams{AggregateType: "run", AggregateID: mustUUID(t, runID)})
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func eventOfType(t *testing.T, pool *pgxpool.Pool, runID, eventType string) gen.OutboxEvent {
	t.Helper()
	for _, e := range outboxFor(t, pool, runID) {
		if e.EventType == eventType {
			return e
		}
	}
	t.Fatalf("run %s has no %s event", runID, eventType)
	return gen.OutboxEvent{}
}

func insertAgedOutboxEvent(t *testing.T, pool *pgxpool.Pool, workspace pgtype.UUID, publishedAt time.Time, deadAt *time.Time) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO outbox_events (
			event_type, event_version, occurred_at, correlation_id, workspace_id,
			aggregate_type, aggregate_id, payload, published_at, dead_lettered_at
		)
		SELECT 'run.cleanup_cleaned', 1, $1::timestamptz, g.id, $2::uuid, 'run', g.id,
		       '{}'::jsonb, $1::timestamptz, $3::timestamptz
		FROM (SELECT gen_random_uuid() AS id) g
		RETURNING event_id`,
		publishedAt, workspace, deadAt,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func outboxRowExists(t *testing.T, pool *pgxpool.Pool, eventID pgtype.UUID) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM outbox_events WHERE event_id = $1)", eventID,
	).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func unpublishedCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	events, err := gen.New(pool).ListUnpublishedOutboxEvents(context.Background(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

func uuidText(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}

type labelledJSON struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

func purgeRunOutputs(t *testing.T, pool *pgxpool.Pool, store objreconcile.ObjectStore) int {
	t.Helper()
	svc := &run.Service{Pool: pool}
	n, err := objreconcile.PurgeExpired(context.Background(), pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ExpiredArtifactCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		svc.MarkRunOutputPurged, svc.GuardArtifactUploadIntentRemoval, 100)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func seedRunOutput(
	t *testing.T, pool *pgxpool.Pool, a *api, workspaceID, runID, fileName string,
	expiresInHours int, deleted bool,
) (pgtype.UUID, string) {
	t.Helper()
	key := "runs/" + runID + "/" + fileName
	a.packages[key] = []byte("output bytes for " + fileName)
	var deletedAt *time.Time
	if deleted {
		now := time.Now()
		deletedAt = &now
	}
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO artifacts (
			workspace_id, run_id, kind, file_name, content_type, size_bytes,
			content_hash, object_key, expires_at, deleted_at
		) VALUES ($1, $2, 'run_output', $3, 'application/octet-stream', 12,
		          'sha256-' || $3, $4, now() + make_interval(hours => $5), $6)
		RETURNING id`,
		mustUUID(t, workspaceID), mustUUID(t, runID), fileName, key,
		expiresInHours, deletedAt,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, key
}

func TestExpiredRunOutputsLoseTheirBytesAndNothingInsideItsWindowDoes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "retainer")
	runID := f.start(t).RunID

	expired, expiredKey := seedRunOutput(t, pool, a, f.workspaceID, runID, "expired.txt", -24, false)
	deletedID, deletedKey := seedRunOutput(t, pool, a, f.workspaceID, runID, "deleted.txt", -48, true)
	liveID, liveKey := seedRunOutput(t, pool, a, f.workspaceID, runID, "live.txt", 24, false)
	if _, err := pool.Exec(context.Background(), `INSERT INTO object_reconcile_sightings
		(resource_kind, resource_id, object_key, rounds) VALUES ('artifact', $1, $2, 2)`,
		expired, expiredKey); err != nil {
		t.Fatal(err)
	}

	if n := purgeRunOutputs(t, pool, a.packages); n < 2 {
		t.Fatalf("the sweep purged %d artifacts, want at least this test's expired and previously deleted rows", n)
	}

	if _, ok := a.packages[expiredKey]; ok {
		t.Error("the expired object survived the sweep")
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NOT NULL AND deleted_at IS NULL",
		expired); n != 1 {
		t.Error("the expired row was not marked purged, or it left the history")
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM object_reconcile_sightings WHERE resource_kind = 'artifact' AND resource_id = $1",
		expired); n != 0 {
		t.Error("the expired run output left a stale missing-object sighting")
	}

	if _, ok := a.packages[deletedKey]; ok {
		t.Error("the durable cleanup worklist did not finish a previously failed user deletion")
	}
	if _, ok := a.packages[liveKey]; !ok {
		t.Error("the sweep removed an object still inside its retention window")
	}
	if n := countRows(t, pool, "SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NOT NULL", deletedID); n != 1 {
		t.Error("the deleted row was not marked purged after its object was removed")
	}
	for name, id := range map[string]pgtype.UUID{"live": liveID} {
		if n := countRows(t, pool,
			"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NULL", id); n != 1 {
			t.Errorf("the %s row was marked purged; the sweep must not reach it", name)
		}
	}

	if n := purgeRunOutputs(t, pool, a.packages); n != 0 {
		t.Errorf("the second sweep purged %d artifacts, want 0", n)
	}
	if _, ok := a.packages[liveKey]; !ok {
		t.Error("the second sweep took the live object")
	}
}

func TestRunArtifactDeleteUsesItsAlreadyLockedConnection(t *testing.T) {
	shared := requireDB(t)
	config := shared.Config()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "single-connection-run-artifact-delete")
	runID := f.start(t).RunID
	artifactID, key := seedRunOutput(t, pool, a, f.workspaceID, runID, "delete-me.txt", 24, false)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := a.runs.DeleteArtifact(ctx, identity.Workspace{
		ID: mustUUID(t, f.workspaceID), OwnerUserID: mustUUID(t, f.userID),
	}, mustUUID(t, runID), artifactID); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.packages[key]; ok {
		t.Fatal("deleted run artifact bytes survived")
	}
}
