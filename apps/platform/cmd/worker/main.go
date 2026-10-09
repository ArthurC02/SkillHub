package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func cleanModeRefusal() string {
	if os.Getenv("SKILLHUB_CLEAN_MODE") != "1" {
		return ""
	}
	return "SKILLHUB_CLEAN_MODE=1 in cmd/worker: clean mode is a single process " +
		"and cmd/api runs the worker set itself, so a separate " +
		"worker carrying this flag is a copied configuration, not a clean-mode " +
		"deployment. Unset it here, or run only cmd/api."
}

func startupRefusals(providers *run.Registry) []string {
	refusals := append(wiring.PostureFromEnv().WorkerRefusals(), providers.UnauthenticatedProviderRefusals()...)
	if reason := cleanModeRefusal(); reason != "" {
		refusals = append(refusals, reason)
	}
	if _, err := capacity.ParseRestoreRate(os.Getenv(capacity.RestoreRateEnv)); err != nil {
		refusals = append(refusals, err.Error())
	}
	if _, err := wiring.JudgePanelFromEnv(); err != nil {
		refusals = append(refusals, err.Error())
	}
	return refusals
}

func prepareQueueSchema(ctx context.Context, pool *pgxpool.Pool) bool {
	if err := queue.EnsureSchema(ctx, pool); err != nil {
		slog.Error("queue schema", "error", err)
		return false
	}
	return true
}

func registerPlatformAgents(ctx context.Context, pool *pgxpool.Pool) bool {
	if err := (&operations.Service{Pool: pool}).Register(ctx, operations.Definitions()); err != nil {
		slog.Error("platform agent registration", "error", err)
		return false
	}
	return true
}

func restoreRateFromEnv() capacity.RestoreRate {
	rate, _ := capacity.ParseRestoreRate(os.Getenv(capacity.RestoreRateEnv))
	return rate
}

func judgePanelFromEnv() []string {
	panel, _ := wiring.JudgePanelFromEnv()
	return panel
}

func main() {
	if code := runWorker(); code != 0 {
		os.Exit(code)
	}
}

func workerDatabasePool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := wiring.DatabasePoolConfig(os.Getenv("DATABASE_URL"), wiring.WorkerPoolMaxConns, wiring.WorkerPoolAcquireWait)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func runWorker() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if refusals := startupRefusals(wiring.NewRunRegistryFromEnv()); len(refusals) > 0 {
		for _, reason := range refusals {
			slog.Error("worker refuses to start", "reason", reason)
		}
		return 1
	}

	pool, err := workerDatabasePool(ctx)
	if err != nil {
		slog.Error("database pool", "error", err)
		return 1
	}
	defer pool.Close()

	if !prepareQueueSchema(ctx, pool) {
		return 1
	}

	providers := wiring.NewRunRegistryFromEnv()
	runDeployment := wiring.RunDeploymentFromEnv()
	logSandboxProviders(providers)

	traceSigner, traceBase := traceIngestFromEnv()

	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		return 1
	}

	gateway := wiring.GatewayFromEnv()
	logModelGateway(gateway, runDeployment)

	llm, ok := judgeFromEnv()
	if !ok {
		return 1
	}
	if !registerPlatformAgents(ctx, pool) {
		return 1
	}

	creationLimits, _ := wiring.CreationLimitsFromEnv()
	set, err := worker.BuildWorkers(pool, worker.Deps{
		CreationLimits:     creationLimits,
		Providers:          providers,
		Store:              store,
		Gateway:            gateway,
		RunDeployment:      runDeployment,
		TraceSigner:        traceSigner,
		TraceIngestBaseURL: traceBase,
		LLM:                llm,
		RestoreRate:        restoreRateFromEnv(),
		JudgePanel:         judgePanelFromEnv(),
	})
	if err != nil {
		slog.Error("worker composition", "error", err)
		return 1
	}

	server, started := startServing(ctx, set, creationLimits)
	if !started {
		return 1
	}
	if server != nil {
		defer shutdownCreationListener(server)
	}
	go metrics.Serve(os.Getenv("METRICS_ADDR"))
	go worker.RefreshGauges(ctx, set.Gauges)
	slog.Info("worker started")

	<-ctx.Done()

	queue.Stop(set.Queue)
	slog.Info("worker stopped")
	return 0
}

func logSandboxProviders(providers *run.Registry) {
	names := make([]string, 0, len(providers.Providers))
	for _, p := range providers.Providers {
		names = append(names, p.Name())
	}
	if len(names) == 0 {
		slog.Warn("no sandbox provider configured; runs will fail at dispatch")
		return
	}
	slog.Info("sandbox providers configured", "providers", names)
}

func traceIngestFromEnv() (*trace.Signer, string) {
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	traceBase := os.Getenv("SKILLHUB_TRACE_INGEST_URL")
	if !traceSigner.Enabled() || traceBase == "" {
		slog.Warn("trace ingestion not configured; sandboxes will be dispatched with no trace destination",
			"has_secret", traceSigner.Enabled(), "has_url", traceBase != "")
	}
	return traceSigner, traceBase
}

func logModelGateway(gateway *run.Gateway, runDeployment run.Deployment) {
	if gateway == nil {
		slog.Warn("no model gateway configured; runs will be dispatched with no model credential")
		return
	}

	slog.Info("model gateway configured", "sandbox_base_url", runDeployment.GatewayURL, "model", runDeployment.Model)
}

func judgeFromEnv() (*llmclient.Client, bool) {
	llmURL := os.Getenv("LLM_SERVICE_URL")
	if llmURL == "" {
		slog.Warn("LLM_SERVICE_URL not set; evaluations will be recorded as failed with no task verdict")
		return nil, true
	}
	token := os.Getenv("LLM_SERVICE_TOKEN")
	if token == "" {
		slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
		return nil, false
	}
	llm := wiring.LLMClient(llmURL, token)
	slog.Info("judge service configured", "url", llmURL)
	return llm, true
}

const (
	creationListenerReadHeaderTimeout = 5 * time.Second
	creationListenerIdleTimeout       = 30 * time.Second
)

func startServing(ctx context.Context, set *worker.Set, creationLimits creation.Limits) (*http.Server, bool) {
	server, err := startCreationListener(set, creationLimits)
	if err != nil {
		slog.Error("creation internal listener could not start", "error", err)
		return nil, false
	}
	if err := set.Queue.Start(ctx); err != nil {
		slog.Error("queue start", "error", err)
		if server != nil {
			shutdownCreationListener(server)
		}
		return nil, false
	}
	return server, true
}

func startCreationListener(set *worker.Set, creationLimits creation.Limits) (*http.Server, error) {
	addr, token := os.Getenv("CREATION_WORKER_INTERNAL_ADDR"), os.Getenv("CREATION_WORKER_INTERNAL_TOKEN")
	if addr == "" || token == "" || !creationLimits.Valid() {
		return nil, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Addr: addr, Handler: set.Creation.TransientHandler(token), ReadHeaderTimeout: creationListenerReadHeaderTimeout, ReadTimeout: 10 * time.Second, IdleTimeout: creationListenerIdleTimeout}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("creation internal listener failed", "addr", addr, "error", err)
		}
	}()
	return server, nil
}

func shutdownCreationListener(server *http.Server) {
	stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(stop)
}
