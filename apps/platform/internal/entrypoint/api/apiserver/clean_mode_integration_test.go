package apiserver_test

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

func cleanModePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv(dbURLEnv))
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCleanModeDrainsTheOutboxOnOneConnection(t *testing.T) {
	seed := requireDB(t)
	pool := cleanModePool(t)
	ctx := context.Background()

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

	set, err := worker.BuildWorkers(pool, worker.Deps{Store: store, PollOnly: true})
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}
	if err := set.Queue.Start(ctx); err != nil {
		t.Fatalf("queue start: %v", err)
	}
	t.Cleanup(func() { queue.Stop(set.Queue) })

	runID := uuid.NewString()
	if _, err := seed.Exec(ctx, `
		INSERT INTO outbox_events (event_type, correlation_id, workspace_id, aggregate_type, aggregate_id, payload)
		VALUES ($1, $2::uuid, $3, $4, $2::uuid, '{"status":"succeeded"}'::jsonb)`,
		outbox.RunSucceeded, runID, ws.ID, outbox.AggregateRun); err != nil {
		t.Fatalf("seed outbox event: %v", err)
	}

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
