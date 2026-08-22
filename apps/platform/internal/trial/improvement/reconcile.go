package eval

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	RecoveryInterval   = 5 * time.Minute
	RecoveryStaleAfter = 10 * time.Minute
)

// RecoveryArgs closes stale pending evaluations without ever invoking Judge.
// It is periodic so a recovery attempt that coincides with a database outage is
// retried after the ordinary evaluation job has exhausted its delivery attempts.
type RecoveryArgs struct{}

func (RecoveryArgs) Kind() string { return "recover_pending_evaluations" }

type RecoveryWorker struct {
	river.WorkerDefaults[RecoveryArgs]
	Svc *Service
}

func (w *RecoveryWorker) Work(ctx context.Context, _ *river.Job[RecoveryArgs]) error {
	rows, err := w.Svc.queries().ListStalePendingEvaluations(ctx, gen.ListStalePendingEvaluationsParams{
		StaleBefore: pgtype.Timestamptz{Time: time.Now().Add(-RecoveryStaleAfter), Valid: true},
		ResultLimit: 100,
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Svc.recoverEvaluation(ctx, row.ID, row.WorkspaceID, row.RunID); err != nil {
			return err
		}
	}
	return nil
}
