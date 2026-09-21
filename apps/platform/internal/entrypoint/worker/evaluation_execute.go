package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type EvaluationExecuteWorker struct {
	river.WorkerDefaults[eval.JobArgs]
	Svc *eval.Service
}

func (w *EvaluationExecuteWorker) Work(ctx context.Context, job *river.Job[eval.JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Svc.DeliverEvaluation(ctx, workspaceID, runID, job.Attempt > 1)
}
