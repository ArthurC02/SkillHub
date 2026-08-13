// Command worker consumes the Postgres job queue (ADR-010 deployment unit 3).
// Go is the only queue consumer (ADR-016 rule 3). The River client is wired in
// with the first background job; this shell only owns startup and shutdown.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("worker started")
	<-ctx.Done()
	slog.Info("worker stopped")
}
