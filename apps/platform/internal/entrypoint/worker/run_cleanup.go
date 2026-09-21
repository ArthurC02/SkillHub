package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunCleanupArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (RunCleanupArgs) Kind() string { return "run_cleanup" }

type RunCleanupWorker struct {
	river.WorkerDefaults[RunCleanupArgs]
	Runs *run.Service
}

func (w *RunCleanupWorker) Work(ctx context.Context, job *river.Job[RunCleanupArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Runs.CleanRun(ctx, workspaceID, runID)
}
