package trace

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const PartitionedTable = "trace_events"

func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	report, err := partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
	if retention <= 0 {
		return report, err
	}
	return report, errors.Join(err, purgeStragglersBefore(ctx, pool, oldestKeptMonth(now.Add(-retention))))
}

func oldestKeptMonth(cutoff time.Time) time.Time {
	cutoff = cutoff.UTC()
	return time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func purgeStragglersBefore(ctx context.Context, pool *pgxpool.Pool, before time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return err
	}
	if _, err := gen.New(tx).DeleteTraceEventsBefore(ctx, pgconv.Timestamptz(before)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
