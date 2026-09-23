package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
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

func TestCleanModeCanDeliverEvaluationOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)
	a := newAPI(t, pool)
	fake := providertest.New("clean_mode_evaluation", "test-token")
	t.Cleanup(fake.Close)
	a.runs.Providers = run.NewRegistry(fake.Provider())
	f := newFixture(t, a, pool, "clean-mode-evaluation")
	if _, err := pool.Exec(context.Background(),
		`UPDATE test_cases SET acceptance_criteria = '[]'::jsonb WHERE id = $1`, mustUUID(t, f.testCaseID)); err != nil {
		t.Fatalf("clear evaluation criteria: %v", err)
	}
	startWorker(t, a)

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, "succeeded")
	waitForAutomaticEvaluation(t, pool, created.RunID)

	resp, err := f.Get(f.base + "/runs/" + created.RunID + "/evaluation")
	if err != nil {
		t.Fatalf("GET evaluation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET evaluation: got %d, want 200", resp.StatusCode)
	}
	var view struct {
		EvaluationID string `json:"evaluation_id"`
		Status       string `json:"status"`
		Overall      string `json:"overall"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode evaluation: %v", err)
	}
	if view.EvaluationID == "" || view.Status != "completed" || view.Overall != "undetermined" {
		t.Fatalf("evaluation = %+v, want a completed undetermined verdict", view)
	}
}

func TestCleanModeCanAdvanceCreationOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)
	limits := creationLimits()
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/creation/step" {
			http.NotFound(w, r)
			return
		}
		cost := 0.01
		_ = json.NewEncoder(w).Encode(llmclient.CreationStepResponse{
			Outcome: "confirm_brief", Message: "請確認任務與成功條件。",
			Brief: "整理輸入資料，依指定格式輸出摘要。",
			Model: "fixture-model", PromptVersion: "creation-test/v1",
			Usage: &llmclient.GatewayUsage{CostUSD: &cost, CostSource: llmclient.CostSourceGateway},
		})
	}))
	t.Cleanup(model.Close)
	set, err := worker.BuildWorkers(pool, worker.Deps{
		CreationLimits: limits,
		LLM:            &llmclient.Client{BaseURL: model.URL, Token: "test-service"},
	})
	if err != nil {
		t.Fatalf("build creation worker: %v", err)
	}
	set.Creation.IssueKey = func(context.Context, string, string, float64, time.Duration) (string, error) {
		return "test-attempt-key", nil
	}
	set.Creation.RevokeKey = func(context.Context, string) error { return nil }
	workers := river.NewWorkers()
	river.AddWorker(workers, &worker.CreationStepWorker{Svc: set.Creation})
	consumer, err := queue.New(pool, &river.Config{
		Workers: workers,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
	})
	if err != nil {
		t.Fatalf("create creation queue: %v", err)
	}
	if err := consumer.Start(context.Background()); err != nil {
		t.Fatalf("start creation queue: %v", err)
	}
	t.Cleanup(func() { _ = consumer.Stop(context.Background()) })

	packages := packageStore{}
	app, err := apiserver.NewApp(apiserver.Config{
		Pool: pool, Store: packages, OAuth: &identity.GitHubOAuth{}, DevLogin: true,
		GenerateExposed: true, CreationExposed: true, CreationLimits: limits, CleanMode: true,
		CreationTransient: func(context.Context, creation.JobArgs, *creation.Diagram) error { return nil },
	})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	a := &api{Server: server, auth: app.Auth, creditPool: pool, startingCredits: betaGrantCredits}
	c := a.login(t, "clean-mode-creation")

	view := creationPost(t, c, "/creation-sessions", map[string]any{
		"id": uuid.NewString(), "message": "開始創作", "budget_credits": 650,
	}, http.StatusOK)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := c.Get(c.base + "/creation-sessions/" + view.ID)
		if err != nil {
			t.Fatalf("GET creation session: %v", err)
		}
		var current creation.View
		decodeErr := json.NewDecoder(response.Body).Decode(&current)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET creation session: got %d, want 200", response.StatusCode)
		}
		if decodeErr != nil {
			t.Fatalf("decode creation session: %v", decodeErr)
		}
		if current.Snapshot.PendingAction == "confirm_brief" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("creation worker never produced the brief confirmation on one connection")
}

func TestCleanModeCanSearchOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)
	a := newAPI(t, pool)
	c := a.login(t, "clean-mode-search")

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := c.Get(c.base + "/skills/search?q=clean")
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		done <- result{code: resp.StatusCode}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("GET /skills/search: %v", got.err)
		}
		if got.code != http.StatusOK {
			t.Fatalf("GET /skills/search: got %d, want 200", got.code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GET /skills/search never returned on a single-connection pool")
	}
}

func TestCleanModeCanImportOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)
	a := newAPI(t, pool)
	name := uniqueWorklistLabel("clean-mode-import")
	c := a.login(t, name)
	pkg := namedPackage(t, name, false)

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := c.Post(c.base+"/skills/import/upload", "application/zip", bytes.NewReader(pkg))
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		done <- result{code: resp.StatusCode}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("POST /skills/import/upload: %v", got.err)
		}
		if got.code != http.StatusCreated {
			t.Fatalf("POST /skills/import/upload: got %d, want 201", got.code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("POST /skills/import/upload never returned on a single-connection pool")
	}
}

func TestCleanModeCanPackageOnOneConnection(t *testing.T) {
	requireDB(t)
	pool := cleanModePool(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("clean-mode-package"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("clean-mode-package-skill"))

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := c.Post(
			c.base+packagingPath(skillID, versionID),
			"application/json",
			bytes.NewBufferString(`{"target":"standard"}`),
		)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		done <- result{code: resp.StatusCode}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("POST packaging: %v", got.err)
		}
		if got.code != http.StatusCreated {
			t.Fatalf("POST packaging: got %d, want 201", got.code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("POST packaging never returned on a single-connection pool")
	}
}
