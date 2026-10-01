package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const SweepClaimLease = 15 * time.Minute

func New(pool *pgxpool.Pool, cfg *river.Config) (*river.Client[pgx.Tx], error) {
	if cfg == nil {
		cfg = &river.Config{}
	}
	return river.NewClient(riverpgxv5.New(pool), cfg)
}

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	lockSession, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		return err
	}
	defer func() { _ = lockSession.Close(context.WithoutCancel(ctx)) }()
	if err := gen.New(lockSession).LockQueueSchema(ctx); err != nil {
		return err
	}
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

const StopTimeout = 30 * time.Second

func Stop(client *river.Client[pgx.Tx]) {
	ctx, cancel := context.WithTimeout(context.Background(), StopTimeout)
	defer cancel()
	if err := client.Stop(ctx); err == nil {
		return
	} else {
		slog.Warn("queue did not stop gracefully; cancelling the jobs still running",
			"error", err, "waited", StopTimeout)
	}
	cancelCtx, cancelStop := context.WithTimeout(context.Background(), StopTimeout)
	defer cancelStop()
	if err := client.StopAndCancel(cancelCtx); err != nil {
		slog.Error("queue stop", "error", err)
	}
}
