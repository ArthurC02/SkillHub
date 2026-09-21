package worker

import (
	"context"

	"github.com/riverqueue/river"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunSuperviseArgs struct{}

func (RunSuperviseArgs) Kind() string { return "run_supervise" }

type RunSuperviseWorker struct {
	river.WorkerDefaults[RunSuperviseArgs]
	Runs *run.Service
}

func (w *RunSuperviseWorker) Work(ctx context.Context, _ *river.Job[RunSuperviseArgs]) error {
	return w.Runs.Supervise(ctx)
}
