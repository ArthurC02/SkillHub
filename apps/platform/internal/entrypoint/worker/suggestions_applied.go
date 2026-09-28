package worker

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type SuggestionsAppliedWorker struct {
	river.WorkerDefaults[eval.SuggestionsAppliedArgs]
	Svc *eval.Service
}

func (w *SuggestionsAppliedWorker) Work(ctx context.Context, job *river.Job[eval.SuggestionsAppliedArgs]) error {
	if lastTry := job.Attempt >= job.MaxAttempts; lastTry {
		return w.Svc.ConsumeSuggestionsAppliedOnLastTry(ctx, job.Args)
	}
	return w.Svc.ConsumeSuggestionsApplied(ctx, job.Args)
}
