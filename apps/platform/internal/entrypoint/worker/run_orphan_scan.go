package worker

import (
	"context"

	"github.com/riverqueue/river"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunOrphanScanArgs struct{}

func (RunOrphanScanArgs) Kind() string { return run.OrphanScanJobKind }

type RunOrphanScanWorker struct {
	river.WorkerDefaults[RunOrphanScanArgs]
	Runs *run.Service
}

func (w *RunOrphanScanWorker) Work(ctx context.Context, _ *river.Job[RunOrphanScanArgs]) error {
	return w.Runs.ScanOrphans(ctx)
}
