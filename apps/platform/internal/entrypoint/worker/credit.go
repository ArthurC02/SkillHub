package worker

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
)

type CreditRecomputeWorker struct {
	river.WorkerDefaults[wiring.CreditRecomputeArgs]
	Svc *credit.Service
}

func (w *CreditRecomputeWorker) Work(ctx context.Context, job *river.Job[wiring.CreditRecomputeArgs]) error {
	command := job.Args.Command()
	window := time.Duration(command.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}
	_, err := w.Svc.RecomputeStatistics(ctx, command.StatKind, window)
	return err
}
