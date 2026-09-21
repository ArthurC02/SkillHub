package worker

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
)

type CreationStepWorker struct {
	river.WorkerDefaults[wiring.CreationStepArgs]
	Svc *creation.Service
}

func (w *CreationStepWorker) Work(ctx context.Context, job *river.Job[wiring.CreationStepArgs]) error {
	return w.Svc.Step(ctx, job.Args.Command(), nil)
}

func (*CreationStepWorker) Timeout(*river.Job[wiring.CreationStepArgs]) time.Duration {
	return 3 * time.Minute
}

type CreationExpiryWorker struct {
	river.WorkerDefaults[wiring.CreationExpiryArgs]
	Svc *creation.Service
}

func (w *CreationExpiryWorker) Work(ctx context.Context, _ *river.Job[wiring.CreationExpiryArgs]) error {
	return w.Svc.RecoverExpired(ctx)
}
