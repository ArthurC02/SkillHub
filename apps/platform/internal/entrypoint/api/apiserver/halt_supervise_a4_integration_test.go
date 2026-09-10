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

func TestAP1OnTheOnlyProviderRefusesRunCreationLikeAPoolHaltDoes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)

	resetDetectionInputs(t, pool)
	f := newFixture(t, a, pool, "alice-single-node-halt")
	hash := f.confirmPermissions(t)

	if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
		t.Fatalf("precondition: creating a run before the halt got %d (%s)", code, view.Error)
	}

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

func TestAFailingSuperviseRunDoesNotSwitchOffTheP1Detectors(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	resetDetectionInputs(t, pool)

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `TRUNCATE trace_events`); err != nil {
			t.Fatalf("clearing the seeded masking evidence: %v", err)
		}
	})
	c := a.login(t, "supervise-detectors")
	skillID := seedSkill(t, pool, c.workspaceID, "supervise-detectors-skill")
	runID := seedRun(t, pool, c.workspaceID, skillID)
	ctx := context.Background()

	for i, age := range []time.Duration{30 * time.Minute, 90 * time.Minute} {
		seedTraceEvent(t, pool, c.workspaceID, runID, i+1, time.Now().Add(-age), `[]`)
	}

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

		`SELECT reason FROM dispatch_halts WHERE provider = '' AND source = $1 AND lifted_at IS NULL`,
		run.HaltSourceIncident,
	).Scan(&reason); err != nil {
		t.Fatalf("no fleet-wide P1 was declared: the masking detector never ran, because one run failed to supervise (%v)", err)
	}
	if !strings.Contains(reason, "TraceMaskingStopped") {
		t.Errorf("halt reason = %q, want the TraceMaskingStopped criterion", reason)
	}
}
