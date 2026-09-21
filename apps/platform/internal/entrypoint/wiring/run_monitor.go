package wiring

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const reconcilerLastRun = `SELECT max(coalesce(finalized_at, attempted_at)) FROM river_job WHERE kind = $1`

func LastOrphanScan(pool *pgxpool.Pool) func(context.Context) (time.Time, bool, error) {
	return func(ctx context.Context) (time.Time, bool, error) {
		var last pgtype.Timestamptz
		if err := pool.QueryRow(ctx, reconcilerLastRun, run.OrphanScanJobKind).Scan(&last); err != nil {
			return time.Time{}, false, err
		}
		return last.Time, last.Valid, nil
	}
}
