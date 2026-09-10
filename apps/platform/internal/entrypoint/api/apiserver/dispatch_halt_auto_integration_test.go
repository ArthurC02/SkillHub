package apiserver_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func resetDetectionInputs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	exec := func(stmt string) {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, stmt := range []string{
		// TRUNCATE, not DELETE: the immutability trigger fires per row on
		// UPDATE OR DELETE and never fires on TRUNCATE.
		`TRUNCATE trace_events`,
		`DELETE FROM dispatch_halts`,
		`DELETE FROM river_job`,
	} {
		exec(stmt)
	}

	t.Cleanup(func() { exec(`DELETE FROM dispatch_halts`) })
}

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

	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted with no trace events at all (source=%q)", source)
	}

	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 1, time.Now().Add(-3*time.Hour), `[]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted on events outside the window entirely (source=%q)", source)
	}

	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 2, time.Now().Add(-30*time.Minute), `[]`)
	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 3, time.Now().Add(-time.Minute), `[]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted on one hour of traffic; the rule holds its expression for a second hour (source=%q)", source)
	}

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

	for i := 0; i < 3; i++ {
		sweep()
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four sweeps = %d, want 1", got)
	}

	seedTraceEvent(t, pool, f.workspaceID, run.RunID, 5, time.Now(), `["/arguments/token"]`)
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once masking recovered (source=%q); the release must be a person", source)
	}
}

func TestReconcilerStallHaltsDispatchWithoutAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	resetDetectionInputs(t, pool)
	ctx := context.Background()

	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted before the reconciler had ever run (source=%q)", source)
	}

	seedOrphanScanJob(t, pool, time.Now().Add(-6*time.Minute))
	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted six minutes after a scan, inside the 10 minute window (source=%q)", source)
	}

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

	for i := 0; i < 3; i++ {
		svc.DetectReconcilerStall(ctx)
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four checks = %d, want 1", got)
	}

	seedOrphanScanJob(t, pool, time.Now())
	svc.DetectReconcilerStall(ctx)
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once the reconciler came back (source=%q)", source)
	}
	if got := haltAuditCount(t, pool, "dispatch.resumed") - resumesBefore; got != 0 {
		t.Errorf("dispatch.resumed events = %d; no detector may lift a halt", got)
	}
}

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

	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted with an intact masker (source=%q)", source)
	}

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

	for i := 0; i < 3; i++ {
		sweep()
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four sweeps = %d, want 1", got)
	}

	svc.MaskerCanary = func() []string { return nil }
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once the canary recovered (source=%q); the release must be a person", source)
	}
}

func TestANodeReportingAP02BreachHaltsTheFleetWithoutAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	resetDetectionInputs(t, pool)

	a.runs.Providers.TTL = time.Nanosecond
	ctx := context.Background()
	sweep := func() {
		t.Helper()
		if err := svc.Supervise(ctx); err != nil {
			t.Fatalf("supervisor sweep: %v", err)
		}
	}

	clean := providertest.DefaultCapability(fake.Name)
	clean.Security = newP02Capability("pass", "2 destination(s) unreachable")
	fake.Capability = &clean
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("dispatch halted on a clean P-02 reading (source=%q)", source)
	}

	unknown := providertest.DefaultCapability(fake.Name)
	unknown.Security = newP02Capability("unknown", "no reading taken yet")
	fake.Capability = &unknown
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "" {
		t.Fatalf("an unknown P-02 reading halted the fleet (source=%q); a rebooting node would stop dispatch", source)
	}

	haltsBefore := haltAuditCount(t, pool, "dispatch.halted")
	breached := providertest.DefaultCapability(fake.Name)
	breached.Security = newP02Capability("fail", "reachable from a sandbox: db.internal:5432")
	fake.Capability = &breached
	sweep()
	source, reason, byPlatform := poolHalt(t, pool)
	if source != "p1_incident" {
		t.Fatalf("pool halt source = %q, want p1_incident", source)
	}
	if !strings.Contains(reason, "db.internal:5432") {
		t.Errorf("the halt reason does not name what was reachable: %q", reason)
	}
	if !byPlatform {
		t.Error("declared_by is set; a halt the platform concluded on its own has no human actor")
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Fatalf("dispatch.halted events = %d, want exactly 1", got)
	}

	for i := 0; i < 3; i++ {
		sweep()
	}
	if got := haltAuditCount(t, pool, "dispatch.halted") - haltsBefore; got != 1 {
		t.Errorf("dispatch.halted events after four sweeps = %d, want 1", got)
	}

	fake.Capability = &clean
	sweep()
	if source, _, _ := poolHalt(t, pool); source != "p1_incident" {
		t.Fatalf("the halt lifted itself once the node reported clean (source=%q); the release must be a person", source)
	}
}

func newP02Capability(state, detail string) *run.SecurityCapability {
	return &run.SecurityCapability{P02Probe: &run.P02ProbeReading{
		State: state, CheckedAt: time.Now().UTC(), Detail: detail,
	}}
}
