// Command reindex rebuilds the search projection from skills (INGEST-009).
// The projection is never a source of truth (ADR-010), so this is safe to run
// at any time and is the recovery path if the projection drifts or is lost.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	n, err := gen.New(pool).ReindexAll(ctx)
	if err != nil {
		slog.Error("reindex", "error", err)
		os.Exit(1)
	}
	slog.Info("search projection rebuilt", "documents", n)
}
