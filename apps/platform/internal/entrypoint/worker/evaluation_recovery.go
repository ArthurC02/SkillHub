package worker

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type EvaluationRecoveryArgs struct{}

func (EvaluationRecoveryArgs) Kind() string { return "recover_pending_evaluations" }

type EvaluationRecoveryWorker struct {
	river.WorkerDefaults[EvaluationRecoveryArgs]
	Svc *eval.Service
}

func (w *EvaluationRecoveryWorker) Work(ctx context.Context, _ *river.Job[EvaluationRecoveryArgs]) error {
	return w.Svc.RecoverPending(ctx)
}
