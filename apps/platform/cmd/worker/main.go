package main

import (
	"context"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func cleanModeRefusal() string {
	if os.Getenv("SKILLHUB_CLEAN_MODE") != "1" {
		return ""
	}
	return "SKILLHUB_CLEAN_MODE=1 in cmd/worker: clean mode is a single process " +
		"(ADR-060 決策 6) and cmd/api runs the worker set itself, so a separate " +
		"worker carrying this flag is a copied configuration, not a clean-mode " +
		"deployment. Unset it here, or run only cmd/api."
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if reason := cleanModeRefusal(); reason != "" {
		slog.Error("worker refuses to start", "reason", reason)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := queue.EnsureSchema(ctx, pool); err != nil {
		slog.Error("queue schema", "error", err)
		os.Exit(1)
	}

	providers := run.NewRegistryFromEnv()
	names := make([]string, 0, len(providers.Providers))
	for _, p := range providers.Providers {
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		slog.Warn("no sandbox provider configured; runs will fail at dispatch")
	} else {
		slog.Info("sandbox providers configured", "providers", names)
	}

	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	traceBase := os.Getenv("SKILLHUB_TRACE_INGEST_URL")
	if !traceSigner.Enabled() || traceBase == "" {
		slog.Warn("trace ingestion not configured; sandboxes will be dispatched with no trace destination",
			"has_secret", traceSigner.Enabled(), "has_url", traceBase != "")
	}

	store, err := objstore.FromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
	}

	gateway := run.GatewayFromEnv()
	if gateway == nil {
		slog.Warn("no model gateway configured; runs will be dispatched with no model credential")
	} else {

		slog.Info("model gateway configured", "sandbox_base_url", gateway.SandboxBaseURL, "model", gateway.Model)
	}

	var llm *llmclient.Client
	if llmURL := os.Getenv("LLM_SERVICE_URL"); llmURL != "" {
		token := os.Getenv("LLM_SERVICE_TOKEN")
		if token == "" {
			slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
			os.Exit(1)
		}
		llm = &llmclient.Client{BaseURL: llmURL, Token: token}
		slog.Info("judge service configured", "url", llmURL)
	} else {
		slog.Warn("LLM_SERVICE_URL not set; evaluations will be recorded as failed with no task verdict")
	}

	creationLimits, _ := creation.LimitsFromEnv()
	set, err := worker.BuildWorkers(pool, worker.Deps{
		CreationLimits:     creationLimits,
		Providers:          providers,
		Store:              store,
		Gateway:            gateway,
		TraceSigner:        traceSigner,
		TraceIngestBaseURL: traceBase,
		LLM:                llm,
	})
	if err != nil {
		slog.Error("worker composition", "error", err)
		os.Exit(1)
	}

	if err := set.Queue.Start(ctx); err != nil {
		slog.Error("queue start", "error", err)
		os.Exit(1)
	}
	if addr, token := os.Getenv("CREATION_WORKER_INTERNAL_ADDR"), os.Getenv("CREATION_WORKER_INTERNAL_TOKEN"); addr != "" && token != "" && creationLimits.Valid() {
		server := &http.Server{Addr: addr, Handler: set.Creation.TransientHandler(token), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
		go func() {
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("creation internal listener failed")
			}
		}()
		defer func() {
			stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(stop)
		}()
	}
	go metrics.Serve(os.Getenv("METRICS_ADDR"))
	slog.Info("worker started")

	<-ctx.Done()

	queue.Stop(set.Queue)
	slog.Info("worker stopped")
}
