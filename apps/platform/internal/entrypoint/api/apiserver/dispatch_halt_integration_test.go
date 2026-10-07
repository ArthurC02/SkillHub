package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
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

func haltLiftBody(t *testing.T, halt map[string]any, note string) string {
	t.Helper()
	body := map[string]any{
		"note": note, "halt_id": halt["halt_id"], "generation": halt["generation"],
	}
	if halt["target"] != "pool" {
		body["provider"] = halt["target"]
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func currentHaltLiftBody(t *testing.T, c *client, note string) string {
	t.Helper()
	_, halts := dispatchStatus(t, c)
	if len(halts) != 1 {
		t.Fatalf("active halts = %v, want one", halts)
	}
	return haltLiftBody(t, halts[0], note)
}

func haltAuditCount(t *testing.T, pool *pgxpool.Pool, action string) int {
	t.Helper()
	return countRow(t, pool, "SELECT count(*) FROM audit_events WHERE action = $1", action)
}

func runOrphanScan(t *testing.T, svc *run.Service) {
	t.Helper()
	if err := (&worker.RunOrphanScanWorker{Runs: svc}).Work(context.Background(), nil); err != nil {

		t.Logf("orphan scan reported: %v", err)
	}
}

func letAScanIntervalPass(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, stmt := range []string{
		"UPDATE reconciler_orphan_sightings SET last_seen_at = last_seen_at - interval '1 hour'",
		"UPDATE dispatch_halts SET last_clear_round_at = last_clear_round_at - interval '1 hour' WHERE lifted_at IS NULL",
	} {
		if _, err := pool.Exec(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
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

func TestOperatorLiftRequiresAnObservedHalt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, _ = haltHarness(t, a, pool)
	operator := a.login(t, "operator-observed-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	declareP1Halt(t, operator, "investigating")
	_, halts := dispatchStatus(t, operator)
	before := haltAuditCount(t, pool, "dispatch.resumed")
	for _, body := range []string{
		`{"note":"safe"}`,
		`{"note":"safe","halt_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"note":"safe","halt_id":"invalid","generation":1}`,
		`{"note":"safe","halt_id":"11111111-1111-4111-8111-111111111111","generation":0}`,
	} {
		code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", body)
		message, _ := response["error"].(string)
		if code != http.StatusBadRequest || !strings.HasPrefix(message, "missing_dispatch_halt_observation:") {
			t.Errorf("DELETE %s: got %d (%v), want named 400", body, code, response)
		}
	}
	_, remaining := dispatchStatus(t, operator)
	if len(remaining) != 1 || remaining[0]["halt_id"] != halts[0]["halt_id"] {
		t.Fatalf("missing observations changed active halt: %v", remaining)
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed"); got != before {
		t.Fatalf("missing observations wrote %d resume audits, want none", got-before)
	}
}

func TestOperatorLiftRejectsRedeclaredAndReplacementHalts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, _ = haltHarness(t, a, pool)
	operator := a.login(t, "operator-stale-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	declareP1Halt(t, operator, "investigating")
	_, original := dispatchStatus(t, operator)
	staleBody := haltLiftBody(t, original[0], "safe to resume")
	before := haltAuditCount(t, pool, "dispatch.resumed")

	declareP1Halt(t, operator, "investigating")
	_, redeclared := dispatchStatus(t, operator)
	if redeclared[0]["halt_id"] != original[0]["halt_id"] || redeclared[0]["generation"] != original[0]["generation"].(float64)+1 {
		t.Fatalf("redeclaration identity/generation = %v, original = %v", redeclared[0], original[0])
	}
	if code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", staleBody); code != http.StatusConflict || !strings.HasPrefix(fmt.Sprint(response["error"]), "stale_dispatch_halt:") {
		t.Fatalf("stale redeclaration lift: got %d (%v), want named 409", code, response)
	}
	_, afterStale := dispatchStatus(t, operator)
	if afterStale[0]["generation"] != redeclared[0]["generation"] || haltAuditCount(t, pool, "dispatch.resumed") != before {
		t.Fatal("stale observation lifted or audited a redeclared halt")
	}

	currentBody := haltLiftBody(t, redeclared[0], "safe to resume")
	if code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", currentBody); code != http.StatusNoContent {
		t.Fatalf("matching lift: got %d (%v), want 204", code, response)
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed"); got != before+1 {
		t.Fatalf("matching lift wrote %d audits, want one", got-before)
	}
	declareP1Halt(t, operator, "new incident")
	_, replacement := dispatchStatus(t, operator)
	if replacement[0]["halt_id"] == redeclared[0]["halt_id"] {
		t.Fatalf("replacement kept old halt ID: %v", replacement[0])
	}
	if replacement[0]["generation"] != original[0]["generation"] {
		t.Fatalf("replacement generation = %v, want the former halt's initial %v", replacement[0]["generation"], original[0]["generation"])
	}
	if code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", staleBody); code != http.StatusConflict || !strings.HasPrefix(fmt.Sprint(response["error"]), "stale_dispatch_halt:") {
		t.Fatalf("stale replacement lift: got %d (%v), want named 409", code, response)
	}
	_, remaining := dispatchStatus(t, operator)
	if remaining[0]["halt_id"] != replacement[0]["halt_id"] || haltAuditCount(t, pool, "dispatch.resumed") != before+1 {
		t.Fatal("stale observation lifted or audited a replacement halt")
	}
}

func TestOperatorLiftRejectsAnOrphanHaltUpgradedToP1(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	operator := a.login(t, "operator-upgraded-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	if _, err := svc.DeclareHalt(t.Context(), "", run.HaltSourceOrphanThreshold, "orphan count", pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	_, orphan := dispatchStatus(t, operator)
	staleBody := haltLiftBody(t, orphan[0], "safe to resume")
	before := haltAuditCount(t, pool, "dispatch.resumed")
	declareP1Halt(t, operator, "isolation incident")
	_, incident := dispatchStatus(t, operator)
	if incident[0]["halt_id"] != orphan[0]["halt_id"] || incident[0]["generation"] != orphan[0]["generation"].(float64)+1 || incident[0]["source"] != "p1_incident" {
		t.Fatalf("P1 upgrade = %v, former orphan halt = %v", incident[0], orphan[0])
	}
	code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", staleBody)
	if code != http.StatusConflict || !strings.HasPrefix(fmt.Sprint(response["error"]), "stale_dispatch_halt:") {
		t.Fatalf("stale orphan observation: got %d (%v), want named 409", code, response)
	}
	_, remaining := dispatchStatus(t, operator)
	if remaining[0]["source"] != "p1_incident" || haltAuditCount(t, pool, "dispatch.resumed") != before {
		t.Fatal("old orphan observation lifted or audited the P1 incident")
	}
}

func TestOperatorLiftRollsBackWhenAuditCannotBeWritten(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, _ = haltHarness(t, a, pool)
	operator := a.login(t, "operator-audit-lift")
	a.auth.Operators = map[string]bool{operator.userID: true}
	declareP1Halt(t, operator, "investigating")
	liftBody := currentHaltLiftBody(t, operator, "safe to resume")
	before := haltAuditCount(t, pool, "dispatch.resumed")
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_dispatch_resume_audit ON audit_events`); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_dispatch_resume_audit()`); err != nil {
			t.Error(err)
		}
	})
	for _, statement := range []string{
		`CREATE FUNCTION reject_dispatch_resume_audit() RETURNS trigger LANGUAGE plpgsql AS $$
         BEGIN IF NEW.action = 'dispatch.resumed' THEN RAISE EXCEPTION 'audit refused'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER reject_dispatch_resume_audit BEFORE INSERT ON audit_events
         FOR EACH ROW EXECUTE FUNCTION reject_dispatch_resume_audit()`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if code, response := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt", liftBody); code != http.StatusInternalServerError {
		t.Fatalf("audit refusal: got %d (%v), want 500", code, response)
	}
	_, remaining := dispatchStatus(t, operator)
	if len(remaining) != 1 || haltAuditCount(t, pool, "dispatch.resumed") != before {
		t.Fatal("audit failure committed the lift or wrote a partial audit")
	}
}

func TestP1HaltStopsBothEntryPointsAndPreservesTheScene(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-p1-halt")

	operator := a.login(t, "operator-p1-halt")
	a.auth.Operators = map[string]bool{operator.userID: true}
	sc := haltScene{pool: pool, fake: fake, svc: svc, f: f, operator: operator, ws: mustUUID(t, f.workspaceID)}

	finished := sc.succeededRunHoldingOneSandbox(t)

	hash := f.confirmPermissions(t)
	code, queued := f.startWithHash(t, hash)
	if code != http.StatusCreated {
		t.Fatalf("precondition: creating the queued run got %d (%s)", code, queued.Error)
	}

	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")

	const note = "escape suspicion on the fake fleet; investigating"
	declareP1Halt(t, operator, note)
	sc.assertP1HaltHoldsNewAndQueuedWork(t, hash, queued.RunID)
	sc.assertP1HaltPreservesTheScene(t, finished.RunID)
	sc.assertP1HaltAuditedAndNotSelfReleasing(t, haltsBefore, note)
	liftBody := sc.resumeAndFinishTheHeldWork(t, queued.RunID, finished.RunID)

	resumesBeforeRepeat := haltAuditCount(t, pool, "dispatch.resumed")
	if code, _ := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
		liftBody); code != http.StatusNoContent {
		t.Error("a repeated resume is not a no-op")
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed"); got != resumesBeforeRepeat {
		t.Errorf("a repeated resume wrote %d audits, want none", got-resumesBeforeRepeat)
	}

	for _, body := range []string{`{}`, `{"note":"  "}`, `{"note":"n","provider":"no_such_node"}`} {
		if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/dispatch/halt", body); code != http.StatusBadRequest {
			t.Errorf("PUT %s: got %d, want 400", body, code)
		}
	}
}

type haltScene struct {
	pool     *pgxpool.Pool
	fake     *providertest.Fake
	svc      *run.Service
	f        fixture
	operator *client
	ws       pgtype.UUID
}

func (sc haltScene) succeededRunHoldingOneSandbox(t *testing.T) runView {
	t.Helper()
	finished := sc.f.start(t)
	if err := driveThroughPolls(context.Background(), sc.svc.Drive, sc.ws, mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run before the halt: %v", err)
	}
	if _, view := sc.f.getRun(t, finished.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Fatalf("precondition: run status = %q, want succeeded", view.Status)
	}
	if sc.fake.Live() != 1 {
		t.Fatalf("precondition: %d sandboxes held, want 1", sc.fake.Live())
	}
	return finished
}

func declareP1Halt(t *testing.T, operator *client, note string) {
	t.Helper()
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/dispatch/halt",
		`{"note":"`+note+`"}`); code != http.StatusOK {
		t.Fatalf("operator PUT /admin/dispatch/halt: got %d (%v)", code, body)
	}
	if dispatching, halts := dispatchStatus(t, operator); dispatching || len(halts) != 1 {
		t.Fatalf("status after the halt: dispatching=%v, halts=%v", dispatching, halts)
	} else if halts[0]["source"] != string(run.HaltSourceIncident) || halts[0]["automatic_recovery"] != false {
		t.Errorf("halt reported as %v; a P1 is never lifted automatically", halts[0])
	}
}

func (sc haltScene) assertP1HaltHoldsNewAndQueuedWork(t *testing.T, hash, queuedRunID string) {
	t.Helper()
	if code, view := sc.f.startWithHash(t, hash); code != http.StatusServiceUnavailable {
		t.Errorf("creating a run under a P1 halt: got %d (%s), want 503", code, view.Error)
	}

	if err := driveThroughPolls(context.Background(), sc.svc.Drive, sc.ws, mustUUID(t, queuedRunID)); err != nil {
		t.Fatalf("driving a run under a halt returned an error: %v", err)
	}
	if _, view := sc.f.getRun(t, queuedRunID); view.Status != string(gen.RunStatusQueued) {
		t.Fatalf("the queued run moved to %q under a halt (%s)", view.Status, view.StatusReason)
	}
	if sc.fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d; the halted fleet was handed more work", sc.fake.Dispatches())
	}
}

func (sc haltScene) assertP1HaltPreservesTheScene(t *testing.T, finishedRunID string) {
	t.Helper()
	if err := sc.svc.CleanRun(context.Background(), mustUUID(t, sc.f.workspaceID), mustUUID(t, finishedRunID)); err != nil {
		t.Fatalf("cleanup under a halt returned an error: %v", err)
	}
	if sc.fake.Destroys() != 0 || sc.fake.Live() != 1 {
		t.Errorf("cleanup destroyed the scene under a P1 halt: destroys=%d live=%d",
			sc.fake.Destroys(), sc.fake.Live())
	}
	if _, view := sc.f.getRun(t, finishedRunID); view.CleanupStatus.Value == string(gen.RunCleanupStatusCleaned) {
		t.Error("cleanup_status says cleaned while the sandbox is still standing")
	}

	orphan := sc.fake.Seed("00000000-0000-0000-0000-0000000000aa", "", time.Now().Add(-time.Hour))
	runOrphanScan(t, sc.svc)
	if sc.fake.Destroys() != 0 {
		t.Errorf("the orphan scan destroyed %d sandboxes under a P1 halt", sc.fake.Destroys())
	}
	if n := countRow(t, sc.pool,
		"SELECT count(*) FROM reconciler_orphan_sightings WHERE provider_run_id = $1", orphan); n != 1 {
		t.Errorf("the held scan recorded %d sightings, want 1: the X-04 count must keep running", n)
	}
}

func (sc haltScene) assertP1HaltAuditedAndNotSelfReleasing(t *testing.T, haltsBefore int, note string) {
	t.Helper()
	ctx := context.Background()
	if got := haltAuditCount(t, sc.pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events = %d, want exactly 1", got)
	}
	var actor, target, reason string
	if err := sc.pool.QueryRow(ctx, `
		SELECT actor_user_id::text, metadata->>'target', metadata->>'reason'
		FROM audit_events WHERE action = 'dispatch.halted' ORDER BY id DESC LIMIT 1`).
		Scan(&actor, &target, &reason); err != nil {
		t.Fatal(err)
	}
	if actor != sc.operator.userID || target != "pool" || reason != note {
		t.Errorf("halt audit = actor %s target %s reason %q, want the operator, the pool and their note",
			actor, target, reason)
	}

	for i := 0; i < 3; i++ {
		sc.svc.EvaluateOrphanThresholds(ctx)
	}
	if dispatching, _ := dispatchStatus(t, sc.operator); dispatching {
		t.Fatal("a P1 halt released itself; only a person may resume the fleet")
	}
	if rounds := countRow(t, sc.pool, "SELECT coalesce(max(clear_rounds), 0) FROM dispatch_halts WHERE lifted_at IS NULL AND source = 'p1_incident'"); rounds != 0 {
		t.Errorf("clear rounds under a P1 = %d; only a capacity pause counts its way to recovery", rounds)
	}
}

func (sc haltScene) resumeAndFinishTheHeldWork(t *testing.T, queuedRunID, finishedRunID string) string {
	t.Helper()
	ctx := context.Background()
	resumesBefore := haltAuditCount(t, sc.pool, "dispatch.resumed")
	liftBody := currentHaltLiftBody(t, sc.operator, "investigation closed, nothing escaped")
	if code, body := operatorCall(t, sc.operator, http.MethodDelete, "/admin/dispatch/halt",
		liftBody); code != http.StatusNoContent {
		t.Fatalf("operator DELETE /admin/dispatch/halt: got %d (%v)", code, body)
	}
	if got := haltAuditCount(t, sc.pool, "dispatch.resumed") - resumesBefore; got != 1 {
		t.Errorf("dispatch.resumed events = %d, want exactly 1", got)
	}
	if dispatching, halts := dispatchStatus(t, sc.operator); !dispatching || len(halts) != 0 {
		t.Fatalf("after the resume: dispatching=%v, halts=%v", dispatching, halts)
	}

	if err := driveThroughPolls(ctx, sc.svc.Drive, sc.ws, mustUUID(t, queuedRunID)); err != nil {
		t.Fatalf("driving the queued run after the resume: %v", err)
	}
	if _, view := sc.f.getRun(t, queuedRunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Errorf("the previously queued run is %q after the resume, want succeeded", view.Status)
	}
	if err := sc.svc.CleanRun(ctx, mustUUID(t, sc.f.workspaceID), mustUUID(t, finishedRunID)); err != nil {
		t.Fatalf("cleanup after the resume: %v", err)
	}
	if _, view := sc.f.getRun(t, finishedRunID); view.CleanupStatus.Value != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("cleanup_status = %+v after the resume, want cleaned", view.CleanupStatus)
	}
	return liftBody
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
	if dispatching, _ := dispatchStatus(t, operator); !dispatching {
		t.Fatal("a scan right after the first counted a second round and halted dispatch")
	}

	letAScanIntervalPass(t, pool)
	runOrphanScan(t, svc)
	dispatching, halts := dispatchStatus(t, operator)
	if dispatching {
		t.Fatal("the X-04 threshold was crossed and dispatch was not halted")
	}
	assertSelfClearingThresholdHaltsIncludeThePool(t, halts)
	if haltAuditCount(t, pool, "dispatch.halted") == 0 {
		t.Error("the reconciler halted the fleet without an audit event")
	}

	sc := haltScene{pool: pool, fake: fake, svc: svc, f: f, operator: operator, ws: ws}
	created := sc.runHeldQueuedByTheThresholdHalt(t)

	queuedElsewhere := newFixture(t, a, pool, "alice-x04-halt-second")
	hash := queuedElsewhere.confirmPermissions(t)
	if code, view := queuedElsewhere.startWithHash(t, hash); code != http.StatusCreated {
		t.Errorf("creating a run under an X-04 pause: got %d (%s), want 201", code, view.Error)
	}

	fake.DestroyStatus = 0
	letAScanIntervalPass(t, pool)
	runOrphanScan(t, svc)
	letAScanIntervalPass(t, pool)
	runOrphanScan(t, svc)
	runOrphanScan(t, svc)
	if dispatching, _ := dispatchStatus(t, operator); dispatching {
		t.Fatal("dispatch resumed after a single clear round and a scan right behind it")
	}
	letAScanIntervalPass(t, pool)
	runOrphanScan(t, svc)
	if dispatching, halts := dispatchStatus(t, operator); !dispatching || len(halts) != 0 {
		t.Fatalf("after two clear rounds: dispatching=%v, halts=%v", dispatching, halts)
	}
	if haltAuditCount(t, pool, "dispatch.resumed") == 0 {
		t.Error("the automatic recovery left no audit event")
	}
	if err := driveThroughPolls(ctx, svc.Drive, ws, mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving the run after the automatic recovery: %v", err)
	}
	if _, view := f.getRun(t, created.RunID); view.Status == string(gen.RunStatusQueued) {
		t.Error("the run is still queued after dispatch resumed")
	}
}

func (sc haltScene) runHeldQueuedByTheThresholdHalt(t *testing.T) runView {
	t.Helper()
	created := sc.f.start(t)
	if created.Status != string(gen.RunStatusQueued) {
		t.Fatalf("run status = %q; a capacity pause leaves runs queued (X-04)", created.Status)
	}
	dispatchesBefore := sc.fake.Dispatches()
	if err := driveThroughPolls(context.Background(), sc.svc.Drive, sc.ws, mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving a run under the X-04 halt: %v", err)
	}
	if _, view := sc.f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("the run moved to %q under the X-04 halt (%s)", view.Status, view.StatusReason)
	}
	if sc.fake.Dispatches() != dispatchesBefore {
		t.Error("the reconciler's halt did not reach the scheduler: work was dispatched anyway")
	}
	return created
}

func assertSelfClearingThresholdHaltsIncludeThePool(t *testing.T, halts []map[string]any) {
	t.Helper()
	var sawPool bool
	for _, h := range halts {
		if h["source"] != string(run.HaltSourceOrphanThreshold) {
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
}

func TestAnIncidentTakesOverACapacityPauseAndIsNeverDowngraded(t *testing.T) {
	pool := requireDB(t)
	svc := &run.Service{Pool: pool, Gateway: providertest.NewGateway()}
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
		[]run.HaltSource{run.HaltSourceOrphanThreshold}); err != nil || lifted {
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
		[]run.HaltSource{run.HaltSourceIncident, run.HaltSourceOrphanThreshold}); err != nil || !lifted {
		t.Fatalf("the operator could not release the P1: lifted=%v err=%v", lifted, err)
	}
}

func nil2uuid() pgtype.UUID { return pgtype.UUID{} }

func TestALeakOnAProviderWhoseSlotsAreUnreadableDoesNotDrainIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	operator := a.login(t, "operator-x04-unreadable")
	a.auth.Operators = map[string]bool{operator.userID: true}

	fake.DestroyStatus = http.StatusInternalServerError
	fake.CapabilityStatus = http.StatusServiceUnavailable
	fake.Seed("00000000-0000-0000-0000-0000000000c1", "", time.Now().Add(-time.Hour))

	runOrphanScan(t, svc)
	letAScanIntervalPass(t, pool)
	runOrphanScan(t, svc)
	if dispatching, halts := dispatchStatus(t, operator); !dispatching || len(halts) != 0 {
		t.Fatalf("dispatching=%v halts=%v: one leak drained a node whose slots could not be read, "+
			"as if it declared none", dispatching, halts)
	}
}
