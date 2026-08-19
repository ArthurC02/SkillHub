// 03:SEC-012 (02:SEC-010 的 P1 自動第一動作) and ADR-022 X-04, which are one
// switch. Shared harness lives in authz_integration_test.go (TestMain, migrate,
// requireDB, newAPI, login), run_integration_test.go (fixture, runView,
// clearRunBacklog) and preflight_integration_test.go (confirmPermissions).
//
// These tests deliberately do NOT start a River worker. Every dispatch, cleanup
// and orphan scan is driven by hand, so "the scheduler did not dispatch" is an
// assertion about a call that returned rather than about a timeout, and the two
// entry points SEC-012 names (creation in the API process, dispatch in the worker
// process) are exercised separately.
package identity_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run/providertest"
)

// haltHarness is a fake provider wired into both planes, with no worker running.
func haltHarness(t *testing.T, a *api, pool *pgxpool.Pool) (*providertest.Fake, *run.Service) {
	t.Helper()
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	registry := run.NewRegistry(fake.Provider())
	a.runs.Providers = registry
	return fake, &run.Service{
		Pool: pool, Providers: registry, Store: a.packages, PollInterval: 20 * time.Millisecond,
	}
}

func dispatchStatus(t *testing.T, c *client) (dispatching bool, halts []map[string]any) {
	t.Helper()
	var body struct {
		Dispatching bool             `json:"dispatching"`
		Halts       []map[string]any `json:"halts"`
	}
	if code := getJSON(t, c.Client, c.base+"/admin/dispatch", &body); code != http.StatusOK {
		t.Fatalf("GET /admin/dispatch: got %d, want 200", code)
	}
	return body.Dispatching, body.Halts
}

func haltAuditCount(t *testing.T, pool *pgxpool.Pool, action string) int {
	t.Helper()
	return countRow(t, pool, "SELECT count(*) FROM audit_events WHERE action = $1", action)
}

// runOrphanScan drives one reconciler round through the registered worker, the
// same entry point cmd/worker's periodic job uses.
func runOrphanScan(t *testing.T, svc *run.Service) {
	t.Helper()
	if err := (&run.OrphanScanWorker{Svc: svc}).Work(context.Background(), nil); err != nil {
		// A destroy that fails is an expected part of these tests: the scan reports it
		// and the sighting bookkeeping still happened, which is what the threshold
		// reads. Nothing here is asserted on the error itself.
		t.Logf("orphan scan reported: %v", err)
	}
}

// 02:SEC-011's non-disclosure rule, applied to a switch that stops the whole
// fleet: an endpoint that halts the platform is one whose existence is worth not
// advertising, so it answers 404 to everyone off the roster — not 401, not 403.
func TestDispatchHaltRoutesAreInvisibleWithoutTheOperatorRole(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	member := a.login(t, "member-dispatch-404")
	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, c := range []struct {
		name string
		cl   *client
	}{{"anonymous", anon}, {"member", member}} {
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/admin/dispatch", ""},
			{http.MethodPut, "/admin/dispatch/halt", `{"note":"n"}`},
			{http.MethodDelete, "/admin/dispatch/halt", `{"note":"n"}`},
		} {
			code, _ := operatorCall(t, c.cl, tc.method, tc.path, tc.body)
			if code != http.StatusNotFound {
				t.Errorf("%s %s as %s: got %d, want 404", tc.method, tc.path, c.name, code)
			}
		}
	}
	if n := countRow(t, pool, "SELECT count(*) FROM dispatch_halts WHERE lifted_at IS NULL"); n != 0 {
		t.Fatalf("%d halts are in force after calls that were all refused", n)
	}
}

// The whole of SEC-012 in one pass: the three automatic actions, the two entry
// points, the audit trail, and the release that is not automatic.
func TestP1HaltStopsBothEntryPointsAndPreservesTheScene(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-p1-halt")

	operator := a.login(t, "operator-p1-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	// A run that reached the end before anything was declared. It is the scene:
	// its sandbox is still held, because with no queue client wired nothing has
	// enqueued the cleanup yet.
	finished := f.start(t)
	if err := svc.Drive(ctx, ws, mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run before the halt: %v", err)
	}
	if _, view := f.getRun(t, finished.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Fatalf("precondition: run status = %q, want succeeded", view.Status)
	}
	if fake.Live() != 1 {
		t.Fatalf("precondition: %d sandboxes held, want 1", fake.Live())
	}

	// A run that is queued and has not been dispatched. ADR-022 X-04 and 02:SEC-010
	// agree on this one: it stays where it is.
	hash := f.confirmPermissions(t)
	code, queued := f.startWithHash(t, hash)
	if code != http.StatusCreated {
		t.Fatalf("precondition: creating the queued run got %d (%s)", code, queued.Error)
	}

	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")

	// --- ① declare -----------------------------------------------------------
	const note = "escape suspicion on the fake fleet; investigating"
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/dispatch/halt",
		`{"note":"`+note+`"}`); code != http.StatusOK {
		t.Fatalf("operator PUT /admin/dispatch/halt: got %d (%v)", code, body)
	}
	if dispatching, halts := dispatchStatus(t, operator); dispatching || len(halts) != 1 {
		t.Fatalf("status after the halt: dispatching=%v, halts=%v", dispatching, halts)
	} else if halts[0]["source"] != run.HaltSourceIncident || halts[0]["automatic_recovery"] != false {
		t.Errorf("halt reported as %v; a P1 is never lifted automatically", halts[0])
	}

	// Entry point 1: creation. 503, because a P1 has no automatic release and a
	// queue that may not move until an investigation ends is a worse answer than
	// saying so now.
	if code, view := f.startWithHash(t, hash); code != http.StatusServiceUnavailable {
		t.Errorf("creating a run under a P1 halt: got %d (%s), want 503", code, view.Error)
	}

	// Entry point 2: dispatch. The queued run is left exactly where it is — not
	// failed, because the halt is reversible and failing it would make the user
	// start over.
	if err := svc.Drive(ctx, ws, mustUUID(t, queued.RunID)); err != nil {
		t.Fatalf("driving a run under a halt returned an error: %v", err)
	}
	if _, view := f.getRun(t, queued.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Fatalf("the queued run moved to %q under a halt (%s)", view.Status, view.StatusReason)
	}
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d; the halted fleet was handed more work", fake.Dispatches())
	}

	// --- ③ preserve the scene -------------------------------------------------
	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("cleanup under a halt returned an error: %v", err)
	}
	if fake.Destroys() != 0 || fake.Live() != 1 {
		t.Errorf("cleanup destroyed the scene under a P1 halt: destroys=%d live=%d",
			fake.Destroys(), fake.Live())
	}
	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus == string(gen.RunCleanupStatusCleaned) {
		t.Error("cleanup_status says cleaned while the sandbox is still standing")
	}

	// The orphan scan is the last thing that could destroy evidence, and it stands
	// down too — while still recording what it saw, or the P1 would silently
	// disable the X-04 count as well.
	orphan := fake.Seed("00000000-0000-0000-0000-0000000000aa", "", time.Now().Add(-time.Hour))
	runOrphanScan(t, svc)
	if fake.Destroys() != 0 {
		t.Errorf("the orphan scan destroyed %d sandboxes under a P1 halt", fake.Destroys())
	}
	if n := countRow(t, pool,
		"SELECT count(*) FROM reconciler_orphan_sightings WHERE provider_run_id = $1", orphan); n != 1 {
		t.Errorf("the held scan recorded %d sightings, want 1: the X-04 count must keep running", n)
	}

	// --- the trail ------------------------------------------------------------
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events = %d, want exactly 1", got)
	}
	var actor, target, reason string
	if err := pool.QueryRow(ctx, `
		SELECT actor_user_id::text, metadata->>'target', metadata->>'reason'
		FROM audit_events WHERE action = 'dispatch.halted' ORDER BY id DESC LIMIT 1`).
		Scan(&actor, &target, &reason); err != nil {
		t.Fatal(err)
	}
	if actor != operator.userID || target != "pool" || reason != note {
		t.Errorf("halt audit = actor %s target %s reason %q, want the operator, the pool and their note",
			actor, target, reason)
	}

	// --- the release, which is manual by construction --------------------------
	// An X-04 style recovery cannot touch this halt: three clear reconciler rounds
	// go by and it is still in force (03:SEC-012 「解除不得是自動的」).
	for i := 0; i < 3; i++ {
		svc.EvaluateOrphanThresholds(ctx)
	}
	if dispatching, _ := dispatchStatus(t, operator); dispatching {
		t.Fatal("a P1 halt released itself; only a person may resume the fleet")
	}

	resumesBefore := haltAuditCount(t, pool, "dispatch.resumed")
	if code, body := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
		`{"note":"investigation closed, nothing escaped"}`); code != http.StatusNoContent {
		t.Fatalf("operator DELETE /admin/dispatch/halt: got %d (%v)", code, body)
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed") - resumesBefore; got != 1 {
		t.Errorf("dispatch.resumed events = %d, want exactly 1", got)
	}
	if dispatching, halts := dispatchStatus(t, operator); !dispatching || len(halts) != 0 {
		t.Fatalf("after the resume: dispatching=%v, halts=%v", dispatching, halts)
	}

	// Everything the halt suspended resumes, and nothing it suspended was lost.
	if err := svc.Drive(ctx, ws, mustUUID(t, queued.RunID)); err != nil {
		t.Fatalf("driving the queued run after the resume: %v", err)
	}
	if _, view := f.getRun(t, queued.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Errorf("the previously queued run is %q after the resume, want succeeded", view.Status)
	}
	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("cleanup after the resume: %v", err)
	}
	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("cleanup_status = %q after the resume, want cleaned", view.CleanupStatus)
	}

	// Idempotent in both directions, same rule as the SEC-011 operator routes.
	if code, _ := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
		`{"note":"double check"}`); code != http.StatusNoContent {
		t.Error("a repeated resume is not a no-op")
	}
	// A note is required either way: an operator action nobody can explain later is
	// not a decision (02:SEC-011 理由必填, applied to the same operator surface).
	for _, body := range []string{`{}`, `{"note":"  "}`, `{"note":"n","provider":"no_such_node"}`} {
		if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/dispatch/halt", body); code != http.StatusBadRequest {
			t.Errorf("PUT %s: got %d, want 400", body, code)
		}
	}
}

// ADR-022 X-04 driving the same switch, and the SEC-012 behaviour following from
// it — the bidirectional check that the two triggers really are one mechanism.
//
// Destroys are made to fail, because that is the only way a leak survives a scan
// round: a sighting the next round cannot see is erased, which is what makes
// `rounds` consecutive (0021).
func TestOrphanThresholdMovesTheSameSwitchAndClearsItself(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-x04-halt")

	operator := a.login(t, "operator-x04-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	// Two leaked sandboxes, older than the dispatch-in-flight grace. The fake
	// declares 4 slots, so ADR-022 X-04's node threshold is 50% = 2 and its pool
	// threshold is max(25% of 4, floor 2) = 2: this is exactly the breach.
	fake.DestroyStatus = http.StatusInternalServerError
	for _, id := range []string{
		"00000000-0000-0000-0000-0000000000b1",
		"00000000-0000-0000-0000-0000000000b2",
	} {
		fake.Seed(id, "", time.Now().Add(-time.Hour))
	}

	// Round one sees them for the first time. X-03 counts consecutive rounds, so
	// nothing is halted yet — and that restraint is the point of the threshold.
	runOrphanScan(t, svc)
	if dispatching, _ := dispatchStatus(t, operator); !dispatching {
		t.Fatal("dispatch was halted on a single sighting; X-03 asks for two consecutive rounds")
	}

	// Round two crosses it.
	runOrphanScan(t, svc)
	dispatching, halts := dispatchStatus(t, operator)
	if dispatching {
		t.Fatal("the X-04 threshold was crossed and dispatch was not halted")
	}
	var sawPool bool
	for _, h := range halts {
		if h["source"] != run.HaltSourceOrphanThreshold {
			t.Errorf("halt %v was not attributed to the X-04 threshold", h)
		}
		if h["automatic_recovery"] != true {
			t.Errorf("halt %v claims no automatic recovery; a capacity pause clears itself", h)
		}
		if h["target"] == "pool" {
			sawPool = true
		}
	}
	if !sawPool {
		t.Errorf("halts = %v, want the fleet-wide pause among them", halts)
	}
	if haltAuditCount(t, pool, "dispatch.halted") == 0 {
		t.Error("the reconciler halted the fleet without an audit event")
	}

	// The SEC-012 side follows from the X-04 side, through the one switch: nothing
	// is dispatched, and the run waits rather than failing.
	created := f.start(t)
	if created.Status != string(gen.RunStatusQueued) {
		t.Fatalf("run status = %q; a capacity pause leaves runs queued (ADR-022 X-04)", created.Status)
	}
	dispatchesBefore := fake.Dispatches()
	if err := svc.Drive(ctx, ws, mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving a run under the X-04 halt: %v", err)
	}
	if _, view := f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("the run moved to %q under the X-04 halt (%s)", view.Status, view.StatusReason)
	}
	if fake.Dispatches() != dispatchesBefore {
		t.Error("the reconciler's halt did not reach the scheduler: work was dispatched anyway")
	}

	// ...and unlike a P1, this one does not refuse creation. Two acceptance
	// criteria, obeyed rather than averaged: X-04 says 「Run 停留 queued」 and its
	// release is automatic, so queueing is a wait with an end.
	hash := f.confirmPermissions(t)
	if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
		t.Errorf("creating a run under an X-04 pause: got %d (%s), want 201", code, view.Error)
	}

	// Recovery: two consecutive clear rounds and no fewer (「不做單輪恢復，避免在清理
	// 不穩定時來回抖動」).
	fake.DestroyStatus = 0
	runOrphanScan(t, svc) // still present at the start of this round; now destroyed
	runOrphanScan(t, svc) // gone: first clear round
	if dispatching, _ := dispatchStatus(t, operator); dispatching {
		t.Fatal("dispatch resumed after a single clear round")
	}
	runOrphanScan(t, svc) // second clear round
	if dispatching, halts := dispatchStatus(t, operator); !dispatching || len(halts) != 0 {
		t.Fatalf("after two clear rounds: dispatching=%v, halts=%v", dispatching, halts)
	}
	if haltAuditCount(t, pool, "dispatch.resumed") == 0 {
		t.Error("the automatic recovery left no audit event")
	}
	if err := svc.Drive(ctx, ws, mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving the run after the automatic recovery: %v", err)
	}
	if _, view := f.getRun(t, created.RunID); view.Status == string(gen.RunStatusQueued) {
		t.Error("the run is still queued after dispatch resumed")
	}
}

// 02:SEC-010's escalation rule 「不確定屬 P1 或 P2 時一律以 P1 處理」 as the shared
// switch has to implement it: a P1 takes over a capacity pause on the same target,
// and nothing downgrades it back. Getting this wrong is not cosmetic — the
// reconciler's very next round would hand a security halt an automatic release.
func TestAnIncidentTakesOverACapacityPauseAndIsNeverDowngraded(t *testing.T) {
	pool := requireDB(t)
	svc := &run.Service{Pool: pool}
	ctx := context.Background()
	operator := mustUUID(t, newAPI(t, pool).login(t, "operator-escalation").userID)

	if _, err := svc.DeclareHalt(ctx, "", run.HaltSourceOrphanThreshold, "leaks", nil2uuid()); err != nil {
		t.Fatal(err)
	}
	halt, err := svc.DeclareHalt(ctx, "", run.HaltSourceIncident, "escape suspicion", operator)
	if err != nil {
		t.Fatal(err)
	}
	if halt.Source != run.HaltSourceIncident || halt.Reason != "escape suspicion" {
		t.Fatalf("halt after the P1 = %s/%q, want the incident to have taken over", halt.Source, halt.Reason)
	}

	// The reconciler's own release path cannot touch it, and its next threshold
	// round cannot turn it back into a capacity pause.
	if _, lifted, err := svc.LiftHalt(ctx, "", "clear", nil2uuid(),
		[]string{run.HaltSourceOrphanThreshold}); err != nil || lifted {
		t.Fatalf("the reconciler released a P1: lifted=%v err=%v", lifted, err)
	}
	again, err := svc.DeclareHalt(ctx, "", run.HaltSourceOrphanThreshold, "leaks again", nil2uuid())
	if err != nil {
		t.Fatal(err)
	}
	if again.Source != run.HaltSourceIncident || again.Reason != "escape suspicion" {
		t.Errorf("a threshold breach downgraded the P1 to %s/%q", again.Source, again.Reason)
	}

	if _, lifted, err := svc.LiftHalt(ctx, "", "investigation closed", operator,
		[]string{run.HaltSourceIncident, run.HaltSourceOrphanThreshold}); err != nil || !lifted {
		t.Fatalf("the operator could not release the P1: lifted=%v err=%v", lifted, err)
	}
}

// nil2uuid is the zero UUID audit.Event reads as "platform-initiated", which is
// what the reconciler passes.
func nil2uuid() pgtype.UUID { return pgtype.UUID{} }

// mustRun reads one run row back for the cleanup calls above, which take the row
// rather than an id.
func mustRun(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string) gen.Run {
	t.Helper()
	row, err := gen.New(pool).GetRun(context.Background(), gen.GetRunParams{
		ID: mustUUID(t, runID), WorkspaceID: mustUUID(t, workspaceID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}
