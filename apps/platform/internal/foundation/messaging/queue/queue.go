package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

func New(pool *pgxpool.Pool, cfg *river.Config) (*river.Client[pgx.Tx], error) {
	if cfg == nil {
		cfg = &river.Config{}
	}
	return river.NewClient(riverpgxv5.New(pool), cfg)
}

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
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
