// Two P1 gaps that only show up with a database behind them: a provider-scoped
// halt on a single-provider fleet, and a supervisor sweep whose detectors were
// switched off by an unrelated failure.
//
// Shared harness as in dispatch_halt_integration_test.go: no River worker runs,
// so every sweep here is a call that returned rather than a timeout.
package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// 03:SEC-012 action ① is 「立即停止派送新 Run」. On the fleet shape production
// actually has — SKILLHUB_SANDBOX_PROVIDERS naming one node — a P1 raised against
// that node is the same operational fact as a P1 on the pool: nothing can be
// dispatched either way. The run-creation gate read only the pool row, so the
// node-scoped halt let creation through, and the user got a 201, watched the run
// sit `queued`, and fifteen minutes later saw `timed_out` — with no string
// anywhere on that path mentioning an incident.
//
// halt.go's own comment is the criterion being violated: 「Accepting a run into a
// queue that may not move until a human has finished an investigation is a worse
// answer than saying so now」. TestP1HaltStopsBothEntryPointsAndPreservesTheScene
// covers the pool-level halt; this is the level it did not reach.
func TestAP1OnTheOnlyProviderRefusesRunCreationLikeAPoolHaltDoes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	// Nothing lifts a P1 by itself (03:SEC-012 「解除不得是自動的」), so the harness
	// clears its own fixture afterwards rather than the platform releasing it —
	// the same arrangement, and the same reason, as dispatch_halt_auto's.
	resetDetectionInputs(t, pool)
	f := newFixture(t, a, pool, "alice-single-node-halt")
	hash := f.confirmPermissions(t)

	// The gate is open until the halt exists, so the refusal below cannot be
	// something else about this fixture.
	if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
		t.Fatalf("precondition: creating a run before the halt got %d (%s)", code, view.Error)
	}

	// A node-scoped incident halt on the one configured provider. This is the row
	// `PUT /admin/dispatch/halt` with {"provider":"fake_sandbox"} writes.
	if _, err := svc.DeclareHalt(
		context.Background(), fake.Name, run.HaltSourceIncident,
		"P-02: a sandbox reached an address it must never reach", pgtype.UUID{},
	); err != nil {
		t.Fatal(err)
	}

	code, view := f.startWithHash(t, hash)
	if code == http.StatusCreated {
		t.Fatal("a run was accepted while a P1 held the only provider; it can only sit queued until the hard deadline kills it")
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("creating a run under a node-scoped P1: got %d (%s), want 503 — the same answer the pool-level halt gives",
			code, view.Error)
	}
}

// 02:SEC-010's P1 detectors ride the supervisor sweep, and supervisor.go's own
// comment says why that is allowed: 「a detector must not be able to fail the work
// it rides on」. The direction was reversed — one run whose supervision errored
// returned from the sweep before any detector had looked — so a wedged run
// silenced every P1 criterion this process can conclude on its own, for as long
// as it stayed wedged. A stuck run and a security incident at the same time is
// the case the detectors are worth the most in.
func TestAFailingSuperviseRunDoesNotSwitchOffTheP1Detectors(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	resetDetectionInputs(t, pool)
	// resetDetectionInputs clears the table on the way IN and the halts on the way
	// out, which is enough for a test that is the last word on the fleet. This one
	// leaves redaction-free trace events behind, and they are exactly the evidence
	// detectMaskingStopped reads — so any later test that sweeps would re-declare
	// the P1 and every run creation after it would get a 503 out of nowhere. The
	// fixture takes its own evidence with it.
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `TRUNCATE trace_events`); err != nil {
			t.Fatalf("clearing the seeded masking evidence: %v", err)
		}
	})
	c := a.login(t, "supervise-detectors")
	skillID := seedSkill(t, pool, c.workspaceID, "supervise-detectors-skill")
	runID := seedRun(t, pool, c.workspaceID, skillID)
	ctx := context.Background()

	// The evidence detectMaskingStopped reads: traffic on both sides of the
	// window (its `[1h]` expression and its `for: 1h`), and not one redacted field
	// in any of it. Two counts and not one, because the rule's premise is about
	// volume — see CountTraceMaskingInWindow.
	for i, age := range []time.Duration{30 * time.Minute, 90 * time.Minute} {
		seedTraceEvent(t, pool, c.workspaceID, runID, i+1, time.Now().Add(-age), `[]`)
	}

	// A queue that cannot be reached, so superviseRun's re-enqueue of the still
	// queued run fails. Any failure in that loop would do; this one is the
	// cheapest to arrange and is a real production shape.
	dead, err := pgxpool.New(ctx, "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dead.Close)
	deadQueue, err := queue.New(dead, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.runs.Queue = deadQueue

	if err := a.runs.Supervise(ctx); err == nil {
		t.Fatal("the sweep reported success while its re-enqueue could not reach the queue; " +
			"collecting the errors must not mean swallowing them")
	}

	var reason string
	if err := pool.QueryRow(ctx,
		// The pool row's provider is the empty string (halt.go's haltPool); "pool"
		// is only how haltTarget spells it for the audit trail.
		`SELECT reason FROM dispatch_halts WHERE provider = '' AND source = $1 AND lifted_at IS NULL`,
		run.HaltSourceIncident,
	).Scan(&reason); err != nil {
		t.Fatalf("no fleet-wide P1 was declared: the masking detector never ran, because one run failed to supervise (%v)", err)
	}
	if !strings.Contains(reason, "TraceMaskingStopped") {
		t.Errorf("halt reason = %q, want the TraceMaskingStopped criterion", reason)
	}
}
