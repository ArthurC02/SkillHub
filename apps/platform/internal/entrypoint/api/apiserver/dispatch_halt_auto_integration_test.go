// 03:SEC-012's first verb, 「偵測到」. The switch itself, the operator endpoint and
// the ADR-022 X-04 trigger are covered by dispatch_halt_integration_test.go; what
// is asserted here is the half that has no person in it — two of 02:SEC-010's five
// P1 criteria concluded by the platform and acted on without anybody being paged.
//
// The other three criteria (逃逸疑慮, the P-02 probe, the gVisor advisory cron) are
// signals from outside this process and keep the operator endpoint as their entry
// point, so there is nothing here to assert about them.
//
// Shared harness: authz_integration_test.go (TestMain, migrate, requireDB, newAPI,
// login), run_integration_test.go (fixture), governance_integration_test.go
// (countRow), dispatch_halt_integration_test.go (haltHarness, haltAuditCount).
package apiserver_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// resetDetectionInputs clears what both detectors read, so an assertion here is
// about the rows this test wrote rather than about whatever ran before it.
//
// trace_events is TRUNCATEd and not DELETEd on purpose: 0005's
// trace_events_immutable trigger is BEFORE UPDATE OR DELETE FOR EACH ROW, and a row
// trigger does not fire on TRUNCATE — which is the same reason 0004 can call trace
// retention a DROP PARTITION. Nothing references the table, so it truncates alone.
func resetDetectionInputs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	exec := func(stmt string) {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, stmt := range []string{
		`TRUNCATE trace_events`,
		`DELETE FROM dispatch_halts`,
		`DELETE FROM river_job`,
	} {
		exec(stmt)
	}
	// Both tests here end with the fleet halted, because that is the assertion —
	// nothing lifts a P1 by itself. Dropping the rows afterwards is the test harness
	// resetting its own fixture, not a release path: the platform's only release is
	// LiftHalt, and a test that used it would be asserting the opposite of 03:SEC-012
	// 「解除不得是自動的」.
	t.Cleanup(func() { exec(`DELETE FROM dispatch_halts`) })
}

// seedTraceEvent writes one event straight to the table, bypassing the ingestion
// handler — which is the point: the failure being simulated is the masker's rules
// silently missing everything, and driving it through the working masker would only
// prove the masker works. `masked` is true because 0019's CHECK allows nothing else,
// and that constraint is exactly why an empty masked_fields is the only shape this
// failure can still take.
func seedTraceEvent(t *testing.T, pool *pgxpool.Pool, ws, runID string, seq int, at time.Time, maskedFields string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO trace_events (
			event_id, workspace_id, run_id, attempt, seq, occurred_at,
			event_type, source, masked, masked_fields, payload
		) VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, 'tool_call', 'sandbox', true, $5::jsonb, '{}'::jsonb)`,
		ws, runID, seq, at, maskedFields); err != nil {
		t.Fatalf("seeding a trace event: %v", err)
	}
}

// seedOrphanScanJob records a reconciler round that ran, the way River would.
func seedOrphanScanJob(t *testing.T, pool *pgxpool.Pool, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO river_job (state, kind, args, attempted_at, finalized_at)
		VALUES ('completed', 'run_orphan_scan', '{}'::jsonb, $1, $1)`, at); err != nil {
		t.Fatalf("seeding a river_job row: %v", err)
	}
}

func poolHalt(t *testing.T, pool *pgxpool.Pool) (source, reason string, byPlatform bool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT source, reason, declared_by IS NULL
		FROM dispatch_halts WHERE provider = '' AND lifted_at IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", false
	}
	if err := rows.Scan(&source, &reason, &byPlatform); err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("more than one active pool halt; 0030's partial unique index should make that impossible")
	}
	return source, reason, byPlatform
}

// 02:SEC-010's `TraceMaskingStopped` criterion, acted on rather than announced.
//
// The whole point of the criterion is that this failure is invisible: with 0019's
// CHECK (masked) in force, a broken masker does not produce unmasked rows, it
// produces rows that claim to be masked and had nothing redacted. Events landing
// with no redactions for an hour is the only evidence there is, and NFR-002 has no
// second detector.
func TestMaskingStoppedHaltsDispatchWithoutAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	resetDetectionInputs(t, pool)
	f := newFixture(t, a, pool, "alice-masking-stopped")
	run := f.start(t)
	ctx := context.Background()
	sweep := func() {
		t.Helper()
		if err := svc.Supervise(ctx); err != nil {
			t.Fatalf("supervisor sweep: %v", err)
		}
	}

	// An empty window is a platform with nothing to mask, not a masker that stopped.
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted with no trace events at all (source=%q)", source)
	}

	// Neither is an event older than everything the rule looks at.
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 1, time.Now().Add(-3*time.Hour), `[]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted on events outside the window entirely (source=%q)", source)
	}

	// Nor is a burst inside the last hour with nothing before it. That is the rule's
	// expression satisfied without its `for: 1h`, and it is what a quiet hour that
	// legitimately carried nothing worth redacting looks like — halting the fleet on
	// it is the false positive maskingWindow's comment is about.
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 2, time.Now().Add(-30*time.Minute), `[]`)
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 3, time.Now().Add(-time.Minute), `[]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted on one hour of traffic; the rule holds its expression for a second hour (source=%q)", source)
	}

	// Traffic at both ends and not one field redacted anywhere between. Nobody is
	// asked.
	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 4, time.Now().Add(-90*time.Minute), `[]`)
	sweep()
	source, reason, byPlatform := poolHalt(t, pool)
	if source != "p1_incident" {
		t.Fatalf("pool halt source = %q, want p1_incident", source)
	}
	if reason == "" {
		t.Error("the halt carries no reason; 0030 requires one and an unexplainable halt is not a decision")
	}
	if !byPlatform {
		t.Error("declared_by is set; a halt the platform concluded on its own has no human actor")
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Fatalf("dispatch.halted events = %d, want exactly 1", got)
	}

	// Idempotence. The criterion stays true for as long as the investigation takes,
	// and DeclareHalt is an upsert — so without a guard every 30 second sweep would
	// write another audit event and reset clear_rounds, burying the declaration that
	// mattered under its own repetitions.
	for i := 0; i < 3; i++ {
		sweep()
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four sweeps = %d, want 1", got)
	}

	// 03:SEC-012 「解除不得是自動的」. The condition clearing is not a resumption:
	// the masker redacting again says nothing about whether the secrets already in
	// the table have been dealt with (02:SEC-010 §3's runbook), and letting the
	// trigger decide when service comes back is the thing the requirement forbids.
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 5, time.Now(), `["/arguments/token"]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once masking recovered (source=%q); the release must be a person", source)
	}
}

// 02:SEC-010's 「Reconciler 停擺 > 10 分鐘（ADR-022 X-02）」.
//
// The detector deliberately does not run inside the worker: River's periodic jobs
// are what stops when a worker dies, so a watchdog living there would share the
// fate of the thing it watches. This drives run.Service.DetectReconcilerStall
// directly, which is what cmd/api's ticker calls.
func TestReconcilerStallHaltsDispatchWithoutAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	resetDetectionInputs(t, pool)
	ctx := context.Background()

	// Never scanned at all is not a stall — it is a worker that has not started yet,
	// and it is the case alerts.yml gets for free (a counter with no series cannot
	// satisfy OrphanScanNotRunning either).
	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted before the reconciler had ever run (source=%q)", source)
	}

	// One round ago is inside ADR-022 X-02's two-round tolerance.
	seedOrphanScanJob(t, pool, time.Now().Add(-6*time.Minute))
	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted six minutes after a scan, inside the 10 minute window (source=%q)", source)
	}

	// Two rounds missed. Nobody is asked.
	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")
	resumesBefore := haltAuditCount(t, pool, "dispatch.resumed")
	seedOrphanScanJob(t, pool, time.Now().Add(-11*time.Minute))
	if _, err := pool.Exec(ctx,
		`DELETE FROM river_job WHERE finalized_at > now() - interval '10 minutes'`); err != nil {
		t.Fatal(err)
	}
	svc.DetectReconcilerStall(ctx)
	source, reason, byPlatform := poolHalt(t, pool)
	if source != "p1_incident" {
		t.Fatalf("pool halt source = %q, want p1_incident", source)
	}
	if reason == "" {
		t.Error("the halt carries no reason")
	}
	if !byPlatform {
		t.Error("declared_by is set; a halt the platform concluded on its own has no human actor")
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Fatalf("dispatch.halted events = %d, want exactly 1", got)
	}

	// Idempotence: the reconciler stays down until somebody fixes it, and the ticker
	// keeps firing every OrphanScanInterval meanwhile.
	for i := 0; i < 3; i++ {
		svc.DetectReconcilerStall(ctx)
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four checks = %d, want 1", got)
	}

	// 03:SEC-012 「解除不得是自動的」, again: a reconciler that starts scanning again
	// does not resume dispatch. Whatever leaked while it was down is still out there,
	// and only a person can say it has been dealt with.
	seedOrphanScanJob(t, pool, time.Now())
	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once the reconciler came back (source=%q)", source)
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed") - resumesBefore; got != 0 {
		t.Errorf("dispatch.resumed events = %d; no detector may lift a halt", got)
	}
}

// 02:SEC-010's `TraceMaskingStopped`, concluded from the masker itself rather than
// from a window of traffic.
//
// The traffic reading above needs two hours of events at both ends of a window to
// say anything, and on the only corpus this deployment has ever had — 2,444 dev
// sandbox events with a masked total of zero, all of it synthetic and carrying no
// secrets — it says nothing at all. The canary needs no traffic: it hands the
// masker a synthetic secret of every shape and stops the fleet if one comes back
// intact. Same switch, same audit trail, same absence of an automatic lift.
func TestAFailingMaskerCanaryHaltsDispatchWithoutAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	resetDetectionInputs(t, pool)
	ctx := context.Background()
	sweep := func() {
		t.Helper()
		if err := svc.Supervise(ctx); err != nil {
			t.Fatalf("supervisor sweep: %v", err)
		}
	}
	t.Cleanup(func() { svc.MaskerCanary = nil })

	// The real probe, on this build's real masker. A sweep on a healthy platform is
	// not allowed to stop it, and this is also the assertion that the wiring is not
	// backwards: if the detector concluded on an intact masker, every deployment
	// would halt itself 30 seconds after start-up.
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted with an intact masker (source=%q)", source)
	}

	// One shape stops being redacted. Nobody is asked.
	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")
	svc.MaskerCanary = func() []string { return []string{"openai style key"} }
	sweep()
	source, reason, byPlatform := poolHalt(t, pool)
	if source != "p1_incident" {
		t.Fatalf("pool halt source = %q, want p1_incident", source)
	}
	if !strings.Contains(reason, "openai style key") {
		t.Errorf("the halt reason does not name the shape that survived: %q", reason)
	}
	if !byPlatform {
		t.Error("declared_by is set; a halt the platform concluded on its own has no human actor")
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Fatalf("dispatch.halted events = %d, want exactly 1", got)
	}

	// Idempotence, for the same reason the traffic reading needs it: a broken masker
	// stays broken for as long as the investigation takes, and one audit event per
	// 30 second sweep buries the declaration that mattered.
	for i := 0; i < 3; i++ {
		sweep()
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four sweeps = %d, want 1", got)
	}

	// 03:SEC-012 「解除不得是自動的」. A masker that started redacting again says
	// nothing about the secrets already written while it was not, so the trigger
	// does not get to decide when service comes back.
	svc.MaskerCanary = func() []string { return nil }
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once the canary recovered (source=%q); the release must be a person", source)
	}
}
