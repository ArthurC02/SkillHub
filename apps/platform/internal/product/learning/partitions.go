package analytics

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const PartitionedTable = "analytics_events"

func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	report, err := partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
	if retention <= 0 {
		return report, err
	}
	_, deleteErr := gen.New(pool).DeleteAnalyticsEventsBefore(ctx, pgconv.Timestamptz(now.Add(-retention)))
	return report, errors.Join(err, deleteErr)
}
