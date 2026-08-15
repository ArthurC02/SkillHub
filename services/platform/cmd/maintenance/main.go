// Command maintenance runs the operator-invoked retention and content-health
// jobs. Same shape as cmd/reindex: bounded, idempotent, re-runnable, and
// scheduled by whatever the deployment already uses for cron. Deliberately not
// a scheduler — iron rule 6 keeps the "when" decision outside the code, and a
// second scheduler beside the queue is exactly the moving part nobody pages
// themselves for.
//
//	maintenance purge-accounts   CORE-007: hard delete the private content of
//	                             accounts past their 30-day grace period and
//	                             de-identify what has to be retained. Needs
//	                             DATABASE_URL and object storage.
//	maintenance check-sources    INGEST-010: probe recorded import source URLs
//	                             and mark the ones that no longer resolve.
//	                             Needs DATABASE_URL and network egress.
//
// PURGE_GRACE (Go duration, default 720h) and MAINTENANCE_BATCH (default 100)
// tune one run. A shortened grace applies to requests already in flight.
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/objstore"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("usage: maintenance purge-accounts|check-sources")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "purge-accounts":
		err = purgeAccounts(ctx, pool)
	case "check-sources":
		err = checkSources(ctx, pool)
	default:
		slog.Error("unknown job", "job", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		slog.Error("maintenance job failed", "job", os.Args[1], "error", err)
		os.Exit(1)
	}
}

func purgeAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.New(
		envOr("OBJSTORE_ENDPOINT", "localhost:8333"),
		os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"),
		envOr("OBJSTORE_BUCKET", "skillhub"),
		os.Getenv("OBJSTORE_SSL") == "1",
	)
	if err != nil {
		return err
	}
	svc := &identity.Service{Pool: pool}
	n, err := svc.PurgeExpiredAccounts(ctx, store, grace(), batch())
	if err != nil {
		return err
	}
	slog.Info("account purge complete", "accounts_purged", n)

	// Expired sessions are the other retention sweep this command owns; it is
	// one statement and already idempotent (ADR-020).
	sessions, err := svc.CleanupExpiredSessions(ctx)
	if err != nil {
		return err
	}
	slog.Info("expired sessions removed", "sessions", sessions)
	return nil
}

func checkSources(ctx context.Context, pool *pgxpool.Pool) error {
	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts()}}
	checked, unavailable, err := svc.CheckSources(ctx, batch())
	if err != nil {
		return err
	}
	slog.Info("source check complete", "checked", checked, "unavailable", unavailable)
	return nil
}

func grace() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("PURGE_GRACE")); err == nil && d > 0 {
		return d
	}
	return identity.AccountDeletionGrace
}

func batch() int32 {
	if n, err := strconv.Atoi(os.Getenv("MAINTENANCE_BATCH")); err == nil && n > 0 {
		return int32(n)
	}
	return 100
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
