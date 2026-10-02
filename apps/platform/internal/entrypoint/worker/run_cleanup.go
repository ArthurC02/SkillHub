package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunCleanupWorker struct {
	river.WorkerDefaults[wiring.RunCleanupArgs]
	Runs *run.Service
}

func (w *RunCleanupWorker) Work(ctx context.Context, job *river.Job[wiring.RunCleanupArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Runs.CleanRun(ctx, workspaceID, runID)
}
