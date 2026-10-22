package worker

import (
	"context"
	"log/slog"
	"time"

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

type UnevaluatedRunSweepArgs struct{}

func (UnevaluatedRunSweepArgs) Kind() string { return "enqueue_unevaluated_runs" }

type UnevaluatedRunSweepWorker struct {
	river.WorkerDefaults[UnevaluatedRunSweepArgs]
	Consumer *eval.RunEventConsumer
}

func (w *UnevaluatedRunSweepWorker) Work(ctx context.Context, _ *river.Job[UnevaluatedRunSweepArgs]) error {
	enqueued, err := w.Consumer.EnqueueUnevaluated(ctx, time.Now())
	if enqueued > 0 {
		slog.Warn("finished runs had no evaluation; enqueued them again", "runs", enqueued)
	}
	return err
}
