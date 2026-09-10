package trace

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
)

const PartitionedTable = "trace_events"

func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	return partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
}
