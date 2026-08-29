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
// It is now the only sweep this table has. The bulk DELETE that used to run
// beside it (analytics.PurgeExpired / `maintenance purge-analytics`) is gone:
// 0029 chose partitioning so that "retention becomes DROP PARTITION rather than
// a bulk DELETE", and the DELETE had no supporting index, so on the one place it
// was actually needed — the default partition — it was a full scan of exactly
// the rows partition pruning cannot help with.
//
// What that leaves uncovered is worth stating rather than hiding: rows that
// landed in analytics_events_default are still unreachable by a partition drop,
// and nothing removes them now. This job is what stops them accumulating in the
// first place — miss it for two consecutive months and writes start landing in
// the default — and 0019's drain is the operator procedure for a deployment that
// already has some (see foundation/persistence/partition, createMonth's 23514
// branch).
//
// The window is the same ANALYTICS_RETENTION the writer already gates on: unset
// means this deployment collects nothing at all (ADR-029 決策 5 proposes 180 days
// and PDM-006 has not ratified it), and a deployment that collects nothing has
// nothing to expire. Passing it in rather than reading it here keeps the
// deployment's single source of that number at the composition root.
func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	return partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
}
