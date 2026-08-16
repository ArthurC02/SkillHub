// Command worker consumes the Postgres job queue (ADR-010 deployment unit 3).
// Go is the only queue consumer (ADR-016 rule 3 / iron rule 7): Python is called
// over internal HTTP by a job, it never subscribes to a queue itself.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/outbox"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/objstore"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
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
	store, err := objstore.New(
		addrFromEnv("OBJSTORE_ENDPOINT", "localhost:8333"),
		os.Getenv("OBJSTORE_ACCESS_KEY"), // empty = anonymous, local dev only
		os.Getenv("OBJSTORE_SECRET_KEY"),
		addrFromEnv("OBJSTORE_BUCKET", "skillhub"),
		os.Getenv("OBJSTORE_SSL") == "1",
	)
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

	runs := &run.Service{
		Pool: pool, Providers: providers, Store: store, Gateway: gateway,
		TraceSigner: traceSigner, TraceIngestBaseURL: traceBase,
	}

	workers := river.NewWorkers()
	// Every job kind the platform knows about is registered here. A kind with no
	// worker is not a silent no-op — River fails the job — which is the behaviour
	// we want if a deploy ever drops one.
	river.AddWorker(workers, &run.Worker{Svc: runs})
	river.AddWorker(workers, &run.CleanupWorker{Svc: runs})
	river.AddWorker(workers, &run.OrphanScanWorker{Svc: runs})
	river.AddWorker(workers, &run.SuperviseWorker{Svc: runs})
	river.AddWorker(workers, &outbox.Worker{Pool: pool})

	client, err := queue.New(pool, &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			// One queue until there is a measured reason for more. Concurrency is
			// bounded well below the per-workspace limit of 2 concurrent runs
			// (PDM-005 §5.2); real capacity planning lands with the first load test.
			river.QueueDefault: {MaxWorkers: min(runtime.NumCPU(), 4)},
		},
		// Periodic jobs run on the elected leader only, so several worker processes
		// do not each sweep. RunOnStart is what makes the supervisor the restart
		// recovery path (RUN-008) and not merely a watchdog.
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(run.SuperviseInterval),
				func() (river.JobArgs, *river.InsertOpts) { return run.SuperviseArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(run.OrphanScanInterval),
				func() (river.JobArgs, *river.InsertOpts) { return run.OrphanScanArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(outbox.PublishInterval),
				func() (river.JobArgs, *river.InsertOpts) { return outbox.PublishArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	})
	if err != nil {
		slog.Error("queue client", "error", err)
		os.Exit(1)
	}

	if err := client.Start(ctx); err != nil {
		slog.Error("queue start", "error", err)
		os.Exit(1)
	}
	go metrics.Serve(os.Getenv("METRICS_ADDR"))
	slog.Info("worker started")

	<-ctx.Done()

	// Stop waits for jobs already running to finish; the context they were given
	// is already cancelled, so a job that respects cancellation exits promptly.
	if err := client.Stop(context.Background()); err != nil {
		slog.Error("queue stop", "error", err)
	}
	slog.Info("worker stopped")
}

func addrFromEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
