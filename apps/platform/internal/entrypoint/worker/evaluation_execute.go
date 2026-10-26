package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

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
	if job.Attempt > 1 {
		return deferredJudgement(job, w.Svc.RedeliverEvaluation(ctx, workspaceID, runID))
	}
	return deferredJudgement(job, w.Svc.Evaluate(ctx, workspaceID, runID))
}

func deferredJudgement(job *river.Job[eval.JobArgs], err error) error {
	if !errors.Is(err, eval.ErrJudgementDeferred) {
		return err
	}
	if snoozes(job) >= eval.MaxJudgementDeferrals {
		slog.Warn("evaluation: the model budget stayed spent; the failed judgement stands", "run_id", job.Args.RunID)
		return nil
	}
	return river.JobSnooze(eval.JudgementDeferral)
}

func snoozes(job *river.Job[eval.JobArgs]) int {
	var metadata struct {
		Snoozes int `json:"snoozes"`
	}
	_ = json.Unmarshal(job.Metadata, &metadata)
	return metadata.Snoozes
}

func (*EvaluationExecuteWorker) Timeout(*river.Job[eval.JobArgs]) time.Duration {
	return eval.EvaluationJobTimeout
}
