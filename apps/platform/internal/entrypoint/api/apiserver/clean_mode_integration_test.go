package apiserver_test

// Clean mode (ADR-060 決策 6) end to end, on the one thing its five unit tests
// cannot see: the crossing.
//
// cmd/api/main_test.go pins each of clean mode's consequences at a single
// function boundary — the flag's truth table, the pool's MaxConns, which store
// newStore returns, the static overlay's routing. Every one of them passes while
// the mode is deadlocked, because the defect does not live in any of the four.
// It lives where two of them meet: one connection, and a worker inside the same
// process that wants a second one.
//
// m6/report-inmemory-postgres.md already measured that exact shape ("holding one
// connection while asking a one-connection pool for a second" — 238 seconds of
// deadlock) and the mode's whole design followed from it. The outbox publisher
// reproduced it in the same process a few files over: it held the pool's only
// connection for the length of a delivery, and the delivery's first act is to
// ask the same pool for a connection.
//
// What makes this reachable rather than exotic: a run that finishes is the one
// thing clean mode exists to demonstrate, and `run.succeeded` is one of the two
// event types with a consumer (entrypoint/worker's dispatcher).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

// cleanModePool is the pool cmd/api builds when SKILLHUB_CLEAN_MODE=1: the same
// DSN every other test in this package uses, capped the way applyCleanModePool
// caps it. Its own pool and not testPool's, because MaxConns is the variable
// under test and the shared pool is what the rest of the file needs.
func cleanModePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv(dbURLEnv))
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	cfg.MaxConns = 1 // cmd/api's applyCleanModePool, ADR-060 決策 6
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestCleanModeDrainsTheOutboxOnOneConnection is the crossing test. It is the
// whole of clean mode's process shape — one pool of one connection, the API's
// object graph and the worker's on it together, the in-process object store —
// and it asks the one question the five unit tests cannot: does a finished run
// reach its evaluation?
//
// It fails by timing out rather than by asserting a wrong value, which is what a
// deadlock looks like from outside. Ten seconds is far longer than the work
// needs (the publisher's RunOnStart pass fires within a second of Start) and far
// shorter than the River job timeout the deadlock would otherwise run into.
func TestCleanModeDrainsTheOutboxOnOneConnection(t *testing.T) {
	seed := requireDB(t)
	pool := cleanModePool(t)
	ctx := context.Background()

	// A real workspace: outbox_events.workspace_id is a foreign key, and the
	// event has to be the shape the producer actually writes.
	auth := &identity.Service{Pool: seed}
	user, err := auth.LoginOrSignup(ctx, identity.ExternalIdentity{
		Provider: "github", ProviderUserID: "clean-mode-outbox",
		Email: "clean-mode-outbox@example.test", Name: "Clean", Login: "clean",
	})
	if err != nil {
		t.Fatalf("seed login: %v", err)
	}
	sessionUser, err := auth.UserForToken(ctx, user)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	ws, err := auth.PersonalWorkspace(ctx, sessionUser)
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	store, stopStore, err := objstore.NewInProcess("skillhub-clean-mode-test")
	if err != nil {
		t.Fatalf("objstore.NewInProcess: %v", err)
	}
	t.Cleanup(stopStore)

	app, err := apiserver.NewApp(apiserver.Config{
		Pool: pool, Store: store, Secure: true, CleanMode: true,
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)

	// PollOnly, exactly as cmd/api sets it: with one connection there is none
	// spare for River to LISTEN on.
	set, err := worker.BuildWorkers(pool, worker.Deps{Store: store, PollOnly: true})
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}
	if err := set.Queue.Start(ctx); err != nil {
		t.Fatalf("queue start: %v", err)
	}
	t.Cleanup(func() { queue.Stop(set.Queue) })

	// The event a terminal run announces (contracts/events/domain-events.md §3).
	// Inserted directly rather than by running a run: what is under test is the
	// drain, and a dispatched run would drag a sandbox provider into it.
	runID := uuid.NewString()
	if _, err := seed.Exec(ctx, `
		INSERT INTO outbox_events (event_type, correlation_id, workspace_id, aggregate_type, aggregate_id, payload)
		VALUES ($1, $2::uuid, $3, $4, $2::uuid, '{"status":"succeeded"}'::jsonb)`,
		outbox.RunSucceeded, runID, ws.ID, outbox.AggregateRun); err != nil {
		t.Fatalf("seed outbox event: %v", err)
	}

	// The assertion: the consumer ran. It reacts by enqueuing one evaluation job
	// for this run, and enqueuing is the second of the two things that ask the
	// one-connection pool for a connection while the publisher is holding it.
	deadline := time.Now().Add(10 * time.Second)
	var enqueued int
	for time.Now().Before(deadline) {
		if err := seed.QueryRow(ctx, `
			SELECT count(*) FROM river_job
			WHERE kind = $1 AND args->>'run_id' = $2`, eval.JobArgs{}.Kind(), runID).Scan(&enqueued); err != nil {
			t.Fatalf("count evaluation jobs: %v", err)
		}
		if enqueued > 0 {
			break
		}
		// The other half of the claim, checked while the publisher is mid-pass:
		// a deadlocked publisher holds the process's only connection, and every
		// route that needs one waits behind it. /healthz needs none, so it is
		// the honest liveness question here — "is the HTTP server still
		// answering", not "is the database reachable".
		resp, err := srv.Client().Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz while the outbox drains: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /healthz answered %d while the outbox was draining", resp.StatusCode)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if enqueued == 0 {
		var published, dead int
		_ = seed.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE published_at IS NOT NULL),
			       count(*) FILTER (WHERE dead_lettered_at IS NOT NULL)
			FROM outbox_events WHERE aggregate_id = $1::uuid`, runID).Scan(&published, &dead)
		t.Fatalf(
			"the run's evaluation was never enqueued within 10s (outbox row published=%d dead_lettered=%d). "+
				"A finished run in clean mode gets no verdict: the publisher holds the pool's only "+
				"connection across delivery, and the consumer's first act is to ask the same pool for one",
			published, dead)
	}
}

// The other half of the crossing above, and the half that was never crossed.
//
// TestCleanModeDrainsTheOutboxOnOneConnection asks whether a FINISHED run
// reaches its evaluation. Nothing asked whether a run can be STARTED at all,
// and on 2026-08-30 the answer turned out to be no: POST /skills/{id}/runs
// never returns in clean mode. It was invisible for two reasons that reinforced
// each other. RUN-005 refused every clean-mode dispatch earlier in the chain
// until 04 丙-98 was fixed, so nothing ever reached this code; and every other
// test of this endpoint runs on the shared pool, where a second connection is
// always available and the deadlock cannot form.
//
// A deadlock is what this fails as - a context deadline, not a wrong value - so
// the assertion is the deadline itself. Ten seconds is far longer than one
// insert needs and far shorter than any timeout inside the request path.
func TestCleanModeCanStartARunOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)

	a := newAPI(t, pool)
	fake := providertest.New("clean_mode_fake", "test-token")
	t.Cleanup(fake.Close)
	a.runs.Providers = run.NewRegistry(fake.Provider())
	f := newFixture(t, a, pool, "clean-mode-start")

	hash := f.confirmPermissions(t)
	done := make(chan int, 1)
	go func() {
		code, _ := f.postJSON(t, "/skills/"+f.skillID+"/runs",
			`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
				`","confirmed_summary_hash":"`+hash+`"}`)
		done <- code
	}()

	select {
	case code := <-done:
		if code != http.StatusCreated {
			t.Fatalf("POST run: got %d, want 201", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("POST /skills/{id}/runs never returned on a single-connection pool: " +
			"clean test mode cannot start a run at all (04 丙-99)")
	}
}
