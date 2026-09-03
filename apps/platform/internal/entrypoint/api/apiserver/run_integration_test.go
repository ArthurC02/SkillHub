// Run orchestration integration tests (RUN-001~004). They live in apiserver_test
// with the rest of the database-backed HTTP tests so they serve the real route
// table from apiserver.NewRouter rather than a copy — see authz_integration_test.go
// for the helpers (TestMain, migrate, login) they reuse.
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

// --- seeding -----------------------------------------------------------------
// Runs need a skill version and a test case. Both are seeded straight into the
// database (the version through the existing seedVersion helper) rather than
// through their own endpoints, so these tests fail for run reasons only.

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

// --- HTTP helpers ------------------------------------------------------------

type runView struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
	SkillID      string `json:"skill_id"`
	TestCaseID   string `json:"test_case_id"`
	Provider     string `json:"provider"`
	// `{value,label,note}` since 2026-09-01 (04 丙-115 ②). Decoding it as a
	// string does not error — encoding/json leaves the field at "" — so every
	// assertion below silently compared "" against the class it wanted. That is
	// how CI caught this change and a local `go test ./...` did not: these tests
	// need a database and skip without one.
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

// startWorker runs the real River consumer against the test database, with the
// same worker registration cmd/worker uses.
func startWorker(t *testing.T, a *api) {
	t.Helper()
	startWorkerWith(t, a.runs, a.evaluations)
}

// startWorkerWith is startWorker for a service that has been configured — with a
// provider registry, a shorter poll interval, a lower retry ceiling. Every worker
// cmd/worker registers is registered here too, so the cleanup, orphan-scan and
// supervisor paths are exercised through River rather than called directly.
// evaluator is the composition root's fully wired service, or a copy with only
// the test's judge settings replaced.
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
	// EVAL-001: a finished run gets an evaluation, and River refuses to insert a
	// kind its own Workers bundle does not know. A process that registers some
	// workers but not this one cannot evaluate a run at all — which is why every
	// registration list has to stay in step with cmd/worker, and why this line is
	// not optional test scaffolding.
	evalSvc := *evaluator
	river.AddWorker(workers, &eval.Worker{Svc: &evalSvc})
	// DDD-005: the evaluation job is enqueued by the `run.succeeded`/`run.failed`
	// consumer, not by the terminal transition, so the outbox has to actually
	// drain here for a run to reach a verdict. Wired exactly as cmd/worker wires
	// it, at a test-sized interval.
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
	// The worker needs its own insert-capable client so the jobs it enqueues (the
	// cleanup a terminal transition owes) actually reach the queue.
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

// fixture is a logged-in user with a skill, a version and a test case ready to run.
type fixture struct {
	*client
	skillID, versionID, testCaseID string
}

func newFixture(t *testing.T, a *api, pool *pgxpool.Pool, user string) fixture {
	t.Helper()
	c := a.login(t, user)
	skillID := seedSkill(t, pool, c.workspaceID, user+"-runnable-skill")
	version := seedVersion(t, pool, c.workspaceID, skillID, "hash-"+user)
	// A runnable version has package bytes, and they have to be scannable: SEC-002
	// gate B refuses a version whose static scan cannot be performed at all, so a
	// fixture with an empty object store would be testing that refusal and nothing
	// else. Clean by default; the tests that want a script or a blocking finding
	// overwrite this key with their own package.
	a.packages[version.PackageObjectKey] = cleanPackage(t)
	return fixture{
		client:     c,
		skillID:    skillID,
		versionID:  uuidText(version.ID),
		testCaseID: seedTestCase(t, pool, c.workspaceID, skillID),
	}
}

// cleanPackage is a valid Agent Skills zip with no script and no error-level
// finding — the shape a version that passed import has.
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

// start reads the pre-run permission summary, confirms it, and only then creates
// the run — because that is the only sequence SEC-002 gate B allows (02:TEST-005).
// See preflight_integration_test.go for the assertions on the gate itself.
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

// --- tests -------------------------------------------------------------------

// RUN-001/002/005: a deployment with no sandbox configured at all.
//
// The failure is the point, and so is its shape. Nothing is dispatched, so no
// attempt row is created and there is nothing to clean up: the run goes straight
// from queued to failed, classified as a capability mismatch, with a reason that
// names the missing provider. Before RUN-005 this walked all the way to
// `preparing` and invented an attempt to fail; refusing before dispatch is what
// ADR-004 asks for.
func TestRunFailsImmediatelyWhenNoProviderIsConfigured(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-run-chain")

	created := f.start(t)
	if created.Status != string(gen.RunStatusQueued) {
		t.Fatalf("new run status = %q, want queued", created.Status)
	}
	// RUN-005 selects a provider; until then the run says so instead of naming
	// one it did not choose.
	if created.Provider != "unassigned" {
		t.Errorf("new run provider = %q, want unassigned", created.Provider)
	}
	// The two ids the runs row does not carry, on the create response and on the
	// read alike — a field present on one and absent on the other is one no client
	// can use. `skill_id` is what the EVAL-002 apply call needs; `test_case_id` is
	// the editable draft a re-run would be started from, never the snapshot.
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

	// RUN-006: classified, so the funnel can tell "we had nowhere to run it" from
	// "the skill failed".
	if final.FailureClass.Value != "capability_mismatch" {
		t.Errorf("failure_class = %q, want capability_mismatch", final.FailureClass.Value)
	}

	// RUN-002: every state change recorded, in order, with its reason.
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

	// Nothing was dispatched, so no attempt was invented for a sandbox that never
	// existed (RUN-003).
	if len(final.Attempts) != 0 {
		t.Errorf("attempts = %d, want 0: nothing was ever dispatched", len(final.Attempts))
	}

	// ADR-008: one outbox event per state entered, plus the cleanup that RUN-002
	// requires after every terminal run. Waiting for cleanup first is what makes
	// the event list below deterministic.
	waitForCleanup(t, f.client, created.RunID)
	events := outboxFor(t, pool, created.RunID)
	var types []string
	for _, e := range events {
		types = append(types, e.EventType)
		if got := uuidText(e.CorrelationID); got != created.RunID {
			t.Errorf("event %s correlates on %q, want the platform run_id %q", e.EventType, got, created.RunID)
		}
	}
	// The cleanup event is there because RUN-002 requires a cleanup pass after
	// every terminal run, even one that provisioned nothing.
	wantTypes := []string{"run.queued", "run.failed", "run.cleanup_cleaned"}
	if strings.Join(types, ",") != strings.Join(wantTypes, ",") {
		t.Errorf("outbox event types = %v, want %v", types, wantTypes)
	}
}

// Iron rule 9: the domain change and its event share a transaction, so a failed
// operation leaves neither. Driven through the two ways a run write can fail: a
// creation that is rejected after the run row would have been written, and a
// transition that lost a race.
func TestFailedWritesLeaveNoOutboxEvent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-atomicity")
	q := gen.New(pool)
	ctx := context.Background()

	before := unpublishedCount(t, pool)

	// A run whose test case is not in this workspace: the snapshot insert matches
	// nothing, so the transaction that already inserted nothing else rolls back.
	other := newFixture(t, a, pool, "bob-outbox-atomicity")
	code, _ := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+other.testCaseID+`"}`)
	if code != http.StatusNotFound {
		t.Fatalf("run with another workspace's test case: got %d, want 404", code)
	}
	if after := unpublishedCount(t, pool); after != before {
		t.Errorf("rejected run creation leaked %d outbox events", after-before)
	}

	// A transition that loses the race. The first call moves the run; the second
	// asks for the same move from the same starting state and must change nothing.
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
	// queued (creation) + provisioning (the one that won). The loser wrote nothing.
	if len(history) != 2 {
		t.Errorf("status history has %d rows, want 2", len(history))
	}
}

// failOutboxCommitFor makes any transaction that writes an outbox event for this
// run fail at COMMIT, after every statement in it has already run.
//
// It exists because iron rule 9 is only provable by a failure that lands *after*
// the write under test. outbox.Insert takes a pgx.Tx by type, so passing the pool
// to it does not compile and the rule holds itself up there; audit.Log takes an
// audit.DBTX, which *pgxpool.Pool satisfies as well as the transaction does, so
// that handle can be swapped with nothing to notice. A pool write is its own
// committed row and survives the rollback — which is the difference these tests
// read, and the reason the injection has to be a deferred constraint trigger
// rather than a plain one: in recordCleanup the audit write is the last statement
// before the commit, so nothing earlier in the transaction could fail after it.
//
// Scoped to one run by the WHEN clause, and dropped again, because the whole
// package shares one database with tests that need their events to land.
func failOutboxCommitFor(t *testing.T, pool *pgxpool.Pool, aggregateID pgtype.UUID) {
	t.Helper()
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

// Iron rule 9 on the half TestFailedWritesLeaveNoOutboxEvent cannot reach. Both
// of its scenarios return before record() is ever called — one when LockDraft
// refuses, one when TransitionRun matches no row — so together they prove that an
// early refusal writes nothing, never that what record() already wrote rolls
// back. Handing audit.Log the pool instead of the transaction left the whole
// suite green.
//
// Here the transition gets all the way past its audit write and then cannot
// commit, which is the only situation in which the two handles behave
// differently.
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
	// The trail and the row have to agree: nothing was audited because nothing
	// happened.
	if _, view := f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("status = %q, want queued: a transition that could not commit was applied", view.Status)
	}
}

// ADR-008 「Poison Message 進入隔離佇列並告警，不無限制重送」. Before DDD-012 the
// publisher stopped its pass on a failed delivery and started the next pass at the
// same event, so one event a consumer could never accept held every event
// committed after it — for as long as the consumer stayed broken, which for a
// genuinely undeliverable event is forever.
//
// Two runs, in commit order: the first is undeliverable, the second is the backlog
// behind it. What has to be true at the end is that the first stopped being
// retried and the second got through.
func TestAnUndeliverableEventIsIsolatedAndReleasesTheBacklog(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-poison")

	poisoned := f.start(t)
	behind := f.start(t)
	poisonedID := mustUUID(t, poisoned.RunID)

	// Undeliverable for exactly one run. The outbox backlog is shared with every
	// other test in this package, and failing on all of it would dead-letter events
	// those tests still need.
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

	// Three passes are enough: fail, fail-and-isolate, then the pass that no longer
	// sees it. Looping to five rather than asserting the exact count keeps the test
	// about the outcome, not about how many events other tests left in front.
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
	// Isolated, not delivered and not deleted: the row stays for a human to look at,
	// and nothing in the platform decides on its own that the event did not matter.
	if poison.PublishedAt.Valid {
		t.Error("an event that was never accepted is marked published")
	}
	// The ceiling is a ceiling. Further passes must not keep calling the consumer.
	before := attempts
	if _, err := w.Publish(context.Background()); err != nil {
		t.Fatalf("the pass after isolation still failed: %v", err)
	}
	if attempts != before {
		t.Errorf("the isolated event was delivered %d more times; isolation means it stops", attempts-before)
	}

	// The head of the line was clear, so what was behind it went out.
	if released := eventOfType(t, pool, behind.RunID, outbox.RunQueued); !released.PublishedAt.Valid {
		t.Error("the event committed after the poison never published: the backlog is still blocked")
	}
}

// Retention (ADR-008, contracts/events/domain-events.md §5 缺口 1): 0016 assumed the
// publisher would drop rows it had drained and no DELETE was ever written, so the
// buffer grew without bound. Driven with rows aged on purpose rather than with a
// short window, because a short window here would prune events other tests in this
// package still assert on.
func TestThePublisherPrunesDeliveredEventsButKeepsIsolatedOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-outbox-retention")
	ws := mustUUID(t, f.workspaceID)

	old := time.Now().Add(-8 * 24 * time.Hour)
	stale := insertAgedOutboxEvent(t, pool, ws, old, nil)
	recent := insertAgedOutboxEvent(t, pool, ws, time.Now(), nil)
	// Published *and* isolated is not a state the publisher produces; it is written
	// here precisely because the DELETE's guard against it is the thing under test.
	// Pruning an event nobody could deliver would destroy the only evidence of why.
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

// RUN-002: an illegal pair is refused before the database is touched at all, so a
// caller cannot skip states or rewind a run.
func TestIllegalTransitionIsRefusedWithoutWriting(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-illegal-transition")
	created := f.start(t)

	svc := &run.Service{Pool: pool}
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)
	before := unpublishedCount(t, pool)

	// queued straight to succeeded skips execution and evaluation entirely.
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

// RUN-003: a retry adds a mapping, it does not overwrite the previous one. This is
// the debt 0004 carried by keeping `attempt` and `provider_run_id` on runs itself;
// 0016 moves both onto run_attempts, and this is what that buys.
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
	// The first attempt's provider id survives the second attempt untouched.
	if attempts[0].ProviderRunID == nil || *attempts[0].ProviderRunID != "provider-sandbox-1" {
		t.Errorf("attempt 1 provider_run_id = %v, want provider-sandbox-1", attempts[0].ProviderRunID)
	}
	if attempts[1].ProviderRunID == nil || *attempts[1].ProviderRunID != "provider-sandbox-2" {
		t.Errorf("attempt 2 provider_run_id = %v, want provider-sandbox-2", attempts[1].ProviderRunID)
	}
}

// RUN-004: cancelling records intent; the run keeps its status until something
// actually stops the workload. A queued run has no workload yet, so the worker
// can honour it directly — that is the one case the control plane owns (the rest
// is RUN-006).
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
	// Intent only: the status has not moved.
	if view.Status != string(gen.RunStatusQueued) {
		t.Errorf("status right after cancel = %q, want queued (the workload is not down yet)", view.Status)
	}
	// Idempotent.
	if code, _ := f.postJSON(t, "/runs/"+created.RunID+"/cancel", ""); code != http.StatusAccepted {
		t.Errorf("second cancel: got %d, want 202", code)
	}

	startWorker(t, a)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusCancelled))
	if len(final.Attempts) != 0 {
		t.Errorf("a run cancelled before dispatch created %d attempts, want 0", len(final.Attempts))
	}

	// Cancelling a finished run is a conflict, not a silent rewrite of history.
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

// CORE-006 / WS-006: a run id from another workspace is not a handle on it, and
// existence stays private — 404 everywhere, never 403.
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
	// Bob owns a version and a test case of his own, but not this skill.
	if code, _ := bob.postJSON(t, "/skills/"+alice.skillID+"/runs",
		`{"version_id":"`+bob.versionID+`","test_case_id":"`+bob.testCaseID+`"}`); code != http.StatusNotFound {
		t.Errorf("run on another workspace's skill: got %d, want 404", code)
	}
	// And a version he does own cannot be run under someone else's skill id, nor
	// can Alice's version be run through her skill by him.
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

// --- assertions on raw rows --------------------------------------------------

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

// insertAgedOutboxEvent writes a delivered event with a chosen published_at, which
// is the only way to exercise a seven-day window inside a test. deadAt marks it
// isolated as well — a combination the publisher never produces, written here to
// prove the DELETE refuses to touch it.
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

// labelledJSON is the contract's Labelled as a test decodes it.
type labelledJSON struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// --- retention (SEC-006, PDM-006 §6) ------------------------------------------

// purgeRunOutputs runs the sweep behind `maintenance purge-run-artifacts`, wired
// the way that subcommand wires it: run owns both the worklist and the row
// write, because `artifacts` has two owner contexts and neither may write the
// other's rows (ADR-033). Only the object-then-row ordering is shared with the
// download half.
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

// seedRunOutput writes one run-output row with a chosen expiry and puts bytes
// behind it. Straight into the table because the only producer of these rows is
// a settling run, and a test that had to settle three runs to get three
// artifacts would be testing the driver instead of the sweep.
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

// PDM-006 §6 and the consent document §3 both tell a beta participant a Run
// output is kept 30 days. Before this sweep, "expired" was a value in a column
// and nothing else — the bytes stayed forever, on the one data class the
// participant did not choose to upload.
//
// Three rows, because the two mistakes this sweep can make are opposite ones and
// only the third row catches the second. Expired: bytes go, row stays, so WS-004
// can still say "this expired" rather than "this never existed". Deleted: a
// failed user-triggered object removal leaves durable cleanup work for this
// sweep. Live: inside its
// window, and a sweep that took it would be deleting evidence a run is still
// being judged on.
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

	// Iron rule 9: a second pass finds nothing left to do, and does not fail
	// trying to remove an object that is already gone.
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
