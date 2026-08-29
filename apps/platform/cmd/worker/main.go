// Command worker consumes the Postgres job queue (ADR-010 deployment unit 3).
// Go is the only queue consumer (ADR-016 rule 3 / iron rule 7): Python is called
// over internal HTTP by a job, it never subscribes to a queue itself.
//
// This process's composition root is worker.BuildWorkers
// (internal/entrypoint/worker), not apiserver.NewApp — that one wires the API's
// graph and this one wires the worker's, and the two deployment units share no
// object (ADR-032 §5 實作註記). main() keeps what is genuinely the process:
// reading the environment, starting the queue client and shutting it down.
// BuildWorkers does no I/O, which is what lets internal/entrypoint/worker's own
// tests check the wiring without a database — the check that was missing when
// this file forgot to set run.Service.Queue and every run finished un-cleaned.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	// The sandbox fleet is deployment-static configuration (RUN-005); an empty one
	// is a valid deployment, in which every run fails saying so.
	providers := run.NewRegistryFromEnv()
	names := make([]string, 0, len(providers.Providers))
	for _, p := range providers.Providers {
		names = append(names, p.Name) // names only: the bearer tokens never reach a log
	}
	if len(names) == 0 {
		slog.Warn("no sandbox provider configured; runs will fail at dispatch")
	} else {
		slog.Info("sandbox providers configured", "providers", names)
	}
	// TRACE-002: the worker is what builds a RunRequest, so it is the process
	// that has to be able to mint an ingestion URL. Both halves must be set for
	// collection to happen: the secret this signs with, and the origin an
	// execution node can actually reach the control plane on.
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	traceBase := os.Getenv("SKILLHUB_TRACE_INGEST_URL")
	if !traceSigner.Enabled() || traceBase == "" {
		slog.Warn("trace ingestion not configured; sandboxes will be dispatched with no trace destination",
			"has_secret", traceSigner.Enabled(), "has_url", traceBase != "")
	}
	// SBX-008: the worker is what builds a RunRequest, so it is the process that
	// mints the short-lived object authorizations a sandbox is dispatched with.
	store, err := objstore.FromEnv()
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
	}

	// ADR-017 / iron rule 8: the only model exit. Without it no Virtual Key is
	// minted, the egress allow list stays empty and sandboxes run with no route
	// out - which is a working deployment, just one that cannot call a model.
	gateway := run.GatewayFromEnv()
	if gateway == nil {
		slog.Warn("no model gateway configured; runs will be dispatched with no model credential")
	} else {
		// The address, never the key (iron rule 11).
		slog.Info("model gateway configured", "sandbox_base_url", gateway.SandboxBaseURL, "model", gateway.Model)
	}

	// EVAL-001's task-effect leg lives in apps/llm and is reached over internal
	// HTTP by this worker (iron rule 7). Without LLM_SERVICE_URL there is no judge:
	// runs still get an evaluation row, the deterministic findings are still
	// written, and the row is recorded as `failed` saying the judgement could not
	// be produced - which is a visible state, not a lenient verdict.
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

	set, err := worker.BuildWorkers(pool, worker.Deps{
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
	go metrics.Serve(os.Getenv("METRICS_ADDR"))
	slog.Info("worker started")

	<-ctx.Done()

	// Stop waits for jobs already running to finish; the context they were given
	// is already cancelled, so a job that respects cancellation exits promptly.
	if err := set.Queue.Stop(context.Background()); err != nil {
		slog.Error("queue stop", "error", err)
	}
	slog.Info("worker stopped")
}
