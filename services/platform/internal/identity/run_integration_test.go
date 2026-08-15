// Run orchestration integration tests (RUN-001~004). They live in identity_test
// with the rest of the database-backed HTTP tests so they serve the real route
// table from apiserver.NewRouter rather than a copy — see authz_integration_test.go
// for the helpers (TestMain, migrate, login) they reuse.
package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
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
	Provider     string `json:"provider"`
	Error        string `json:"error"`
	Transitions  []struct {
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
func startWorker(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	workers := river.NewWorkers()
	river.AddWorker(workers, &run.Worker{Svc: &run.Service{Pool: pool}})
	c, err := queue.New(pool, &river.Config{
		Workers: workers,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
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
	return fixture{
		client:     c,
		skillID:    skillID,
		versionID:  uuidText(seedVersion(t, pool, c.workspaceID, skillID, "hash-"+user).ID),
		testCaseID: seedTestCase(t, pool, c.workspaceID, skillID),
	}
}

func (f fixture) start(t *testing.T) runView {
	t.Helper()
	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST run: got %d (%s)", code, view.Error)
	}
	return view
}

// --- tests -------------------------------------------------------------------

// RUN-001/002: the whole chain, end to end. Create a run through the API, let the
// real River worker pick it up, and watch it walk the state machine to a failure
// that says what actually went wrong.
//
// The failure is the point. There is no sandbox provider yet, and the worker says
// so rather than reporting a success nobody executed. When SelfHostedProvider
// lands (RUN-003/SBX-001) this test's expected terminal state changes and nothing
// else does.
func TestRunGoesFromQueuedToFailedWithNoProvider(t *testing.T) {
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

	startWorker(t, pool)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))

	if !strings.Contains(final.StatusReason, "no sandbox provider") {
		t.Errorf("failure reason = %q, want it to name the missing provider", final.StatusReason)
	}

	// RUN-002: every state change recorded, in order, with its reason.
	var path []string
	for _, tr := range final.Transitions {
		path = append(path, tr.To)
		if tr.Reason == "" {
			t.Errorf("transition to %s recorded no reason", tr.To)
		}
	}
	want := []string{"queued", "provisioning", "preparing", "failed"}
	if strings.Join(path, ",") != strings.Join(want, ",") {
		t.Errorf("transition path = %v, want %v", path, want)
	}
	if from := final.Transitions[0].From; from != "" {
		t.Errorf("first transition came from %q, want the run's creation (empty)", from)
	}

	// RUN-003: exactly one attempt, classified.
	if len(final.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(final.Attempts))
	}
	if final.Attempts[0].ErrorClass != "provision" {
		t.Errorf("attempt error_class = %q, want provision", final.Attempts[0].ErrorClass)
	}

	// ADR-008: one outbox event per state entered, all still unpublished because
	// no publisher exists yet (RUN-005).
	events := outboxFor(t, pool, created.RunID)
	var types []string
	for _, e := range events {
		types = append(types, e.EventType)
		if e.PublishedAt.Valid {
			t.Errorf("event %s is marked published, but nothing publishes yet", e.EventType)
		}
		if got := uuidText(e.CorrelationID); got != created.RunID {
			t.Errorf("event %s correlates on %q, want the platform run_id %q", e.EventType, got, created.RunID)
		}
	}
	wantTypes := []string{"run.queued", "run.provisioning", "run.preparing", "run.failed"}
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

	startWorker(t, pool)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusCancelled))
	if len(final.Attempts) != 0 {
		t.Errorf("a run cancelled before dispatch created %d attempts, want 0", len(final.Attempts))
	}

	// Cancelling a finished run is a conflict, not a silent rewrite of history.
	if code, _ := f.postJSON(t, "/runs/"+created.RunID+"/cancel", ""); code != http.StatusConflict {
		t.Errorf("cancel of a finished run: got %d, want 409", code)
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
