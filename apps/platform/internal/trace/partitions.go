package trace

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/partition"
)

// PartitionedTable is the table this context owns (ADR-032 §1, ADR-033:
// trace.sql). It is declared here rather than at the call site because the
// partition rotator is generic and takes whatever name it is handed — this
// constant is what makes "trace owns trace_events" a fact the compiler and the
// reader both see, and it is the reason analytics cannot get this table rotated
// on trace's behalf or the other way round.
const PartitionedTable = "trace_events"

// MaintainPartitions rolls trace_events' monthly partitions forward and drops
// the months that have aged out of `retention`. It is the missing half of what
// db/migrations/0004 called "an operational job, not a migration" and 0019 then
// observed did not exist.
//
// The window is a parameter rather than a constant in this package, and that is
// deliberate even though a number has been written down. 0004's comment,
// m0/pdm-proposals.md and gate-test/consent-and-data-policy.md all say 90 days,
// and all three call it a PDM-006 proposal; 03-work-items.md still records
// PDM-006 as unratified, with Run/Trace/Artifact retention explicitly having no
// value yet. A constant here would turn an unratified proposal into a retention
// this platform actually enforces — irreversibly, since the dropped month does
// not come back. So the deployment supplies it, fail-closed, exactly as
// ANALYTICS_RETENTION already does for the other partitioned table (NFR-002:
// 未定值前不得開始收集, and by the same logic 未定值前不得開始刪除).
//
// TRACE_RETENTION unset therefore means this table's partitions are rolled
// forward by nobody: the caller refuses to run the whole job. That is louder
// than defaulting, which is the point.
func MaintainPartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, error) {
	return partition.MaintainMonthly(ctx, pool, PartitionedTable, now, retention)
}
