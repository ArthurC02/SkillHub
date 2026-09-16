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

	recoveryBatch = 100
)

func statusesAwaitingTheJudge() []string {
	var awaiting []string
	for _, status := range AllStatuses() {
		if status.AwaitsTheJudge() {
			awaiting = append(awaiting, string(status))
		}
	}
	return awaiting
}

type RecoveryArgs struct{}

func (RecoveryArgs) Kind() string { return "recover_pending_evaluations" }

type RecoveryWorker struct {
	river.WorkerDefaults[RecoveryArgs]
	Svc *Service
}

func (w *RecoveryWorker) Work(ctx context.Context, _ *river.Job[RecoveryArgs]) error {
	rows, err := w.Svc.queries().ListStalePendingEvaluations(ctx, gen.ListStalePendingEvaluationsParams{
		AwaitingStatuses: statusesAwaitingTheJudge(),
		StaleBefore:      pgtype.Timestamptz{Time: time.Now().Add(-RecoveryStaleAfter), Valid: true},
		ResultLimit:      recoveryBatch,
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Svc.recoverEvaluation(ctx, row.WorkspaceID, row.ID, row.RunID); err != nil {
			return err
		}
	}
	return nil
}
