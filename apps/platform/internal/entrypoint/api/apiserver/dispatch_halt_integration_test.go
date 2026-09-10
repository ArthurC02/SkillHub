package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func haltHarness(t *testing.T, a *api, pool *pgxpool.Pool) (*providertest.Fake, *run.Service) {
	t.Helper()
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	registry := run.NewRegistry(fake.Provider())
	a.runs.Providers = registry
	a.runs.PollInterval = 20 * time.Millisecond
	return fake, a.runs
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

func runOrphanScan(t *testing.T, svc *run.Service) {
	t.Helper()
	if err := (&run.OrphanScanWorker{Svc: svc}).Work(context.Background(), nil); err != nil {

		t.Logf("orphan scan reported: %v", err)
	}
}

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

func TestP1HaltStopsBothEntryPointsAndPreservesTheScene(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-p1-halt")

	operator := a.login(t, "operator-p1-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

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

	hash := f.confirmPermissions(t)
	code, queued := f.startWithHash(t, hash)
	if code != http.StatusCreated {
		t.Fatalf("precondition: creating the queued run got %d (%s)", code, queued.Error)
	}

	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")

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

	if code, view := f.startWithHash(t, hash); code != http.StatusServiceUnavailable {
		t.Errorf("creating a run under a P1 halt: got %d (%s), want 503", code, view.Error)
	}

	if err := svc.Drive(ctx, ws, mustUUID(t, queued.RunID)); err != nil {
		t.Fatalf("driving a run under a halt returned an error: %v", err)
	}
	if _, view := f.getRun(t, queued.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Fatalf("the queued run moved to %q under a halt (%s)", view.Status, view.StatusReason)
	}
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d; the halted fleet was handed more work", fake.Dispatches())
	}

	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("cleanup under a halt returned an error: %v", err)
	}
	if fake.Destroys() != 0 || fake.Live() != 1 {
		t.Errorf("cleanup destroyed the scene under a P1 halt: destroys=%d live=%d",
			fake.Destroys(), fake.Live())
	}
	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus.Value == string(gen.RunCleanupStatusCleaned) {
		t.Error("cleanup_status says cleaned while the sandbox is still standing")
	}

	orphan := fake.Seed("00000000-0000-0000-0000-0000000000aa", "", time.Now().Add(-time.Hour))
	runOrphanScan(t, svc)
	if fake.Destroys() != 0 {
		t.Errorf("the orphan scan destroyed %d sandboxes under a P1 halt", fake.Destroys())
	}
	if n := countRow(t, pool,
		"SELECT count(*) FROM reconciler_orphan_sightings WHERE provider_run_id = $1", orphan); n != 1 {
		t.Errorf("the held scan recorded %d sightings, want 1: the X-04 count must keep running", n)
	}

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

	if err := svc.Drive(ctx, ws, mustUUID(t, queued.RunID)); err != nil {
		t.Fatalf("driving the queued run after the resume: %v", err)
	}
	if _, view := f.getRun(t, queued.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Errorf("the previously queued run is %q after the resume, want succeeded", view.Status)
	}
	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("cleanup after the resume: %v", err)
	}
	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus.Value != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("cleanup_status = %+v after the resume, want cleaned", view.CleanupStatus)
	}

	if code, _ := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
		`{"note":"double check"}`); code != http.StatusNoContent {
		t.Error("a repeated resume is not a no-op")
	}

	for _, body := range []string{`{}`, `{"note":"  "}`, `{"note":"n","provider":"no_such_node"}`} {
		if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/dispatch/halt", body); code != http.StatusBadRequest {
			t.Errorf("PUT %s: got %d, want 400", body, code)
		}
	}
}

func TestOrphanThresholdMovesTheSameSwitchAndClearsItself(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-x04-halt")

	operator := a.login(t, "operator-x04-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	fake.DestroyStatus = http.StatusInternalServerError
	for _, id := range []string{
		"00000000-0000-0000-0000-0000000000b1",
		"00000000-0000-0000-0000-0000000000b2",
	} {
		fake.Seed(id, "", time.Now().Add(-time.Hour))
	}

	runOrphanScan(t, svc)
	if dispatching, _ := dispatchStatus(t, operator); !dispatching {
		t.Fatal("dispatch was halted on a single sighting; X-03 asks for two consecutive rounds")
	}

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

	hash := f.confirmPermissions(t)
	if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
		t.Errorf("creating a run under an X-04 pause: got %d (%s), want 201", code, view.Error)
	}

	fake.DestroyStatus = 0
	runOrphanScan(t, svc)
	runOrphanScan(t, svc)
	if dispatching, _ := dispatchStatus(t, operator); dispatching {
		t.Fatal("dispatch resumed after a single clear round")
	}
	runOrphanScan(t, svc)
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

func nil2uuid() pgtype.UUID { return pgtype.UUID{} }

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
