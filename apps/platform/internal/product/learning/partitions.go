package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
)

// PartitionedTable is the table this context owns (ADR-032 §1, ADR-033:
// analytics.sql). Declared here for the same reason trace declares its own: the
// rotator is generic, so the name has to come from the owner, and neither owner
// can name the other's table without the import being visible.
const PartitionedTable = "analytics_events"

// MaintainPartitions rolls analytics_events' monthly partitions forward and
// drops the months past `retention`. db/migrations/0029 gave this table the
// shape trace_events already had and named the same missing job.
//
// It does not replace PurgeExpired, and the two are not the same sweep. Rows
// that landed in analytics_events_default — every row written outside a declared
// month, which before this job existed meant everything from 2026-09 onwards —
// are unreachable by a partition drop and are removed by PurgeExpired's bulk
// statement. Rows in a real monthly partition are removed by whichever runs
// first. Running both is correct and neither is redundant; running only this one
// would leave the default growing forever.
//
// The window is the same ANALYTICS_RETENTION the writer already gates on: unset
// means this deployment collects nothing at all (ADR-029 決策 5 proposes 180 days
// and PDM-006 has not ratified it), and a deployment that collects nothing has
// nothing to expire. Passing it in rather than reading it here keeps the
// deployment's single source of that number at the composition root.
func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	return partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
}
