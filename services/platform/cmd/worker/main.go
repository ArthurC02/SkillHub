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

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
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

	workers := river.NewWorkers()
	// Every job kind the platform knows about is registered here. A kind with no
	// worker is not a silent no-op — River fails the job — which is the behaviour
	// we want if a deploy ever drops one.
	river.AddWorker(workers, &run.Worker{Svc: &run.Service{Pool: pool}})

	client, err := queue.New(pool, &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			// One queue until there is a measured reason for more. Concurrency is
			// bounded well below the per-workspace limit of 2 concurrent runs
			// (PDM-005 §5.2); real capacity planning lands with RUN-005.
			river.QueueDefault: {MaxWorkers: min(runtime.NumCPU(), 4)},
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
	slog.Info("worker started")

	<-ctx.Done()

	// Stop waits for jobs already running to finish; the context they were given
	// is already cancelled, so a job that respects cancellation exits promptly.
	if err := client.Stop(context.Background()); err != nil {
		slog.Error("queue stop", "error", err)
	}
	slog.Info("worker stopped")
}
