package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type EvaluationBatchAdvanceWorker struct {
	river.WorkerDefaults[eval.BatchAdvanceArgs]
	Svc *eval.BatchService
}

func (w *EvaluationBatchAdvanceWorker) Work(ctx context.Context, job *river.Job[eval.BatchAdvanceArgs]) error {
	if job.Args.WorkspaceID == "" {
		return w.Svc.AdvanceAllBatches(ctx)
	}
	var workspaceID pgtype.UUID
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Svc.AdvanceBatches(ctx, workspaceID)
}
