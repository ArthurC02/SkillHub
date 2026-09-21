package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunExecuteArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (RunExecuteArgs) Kind() string { return "run_execute" }

type RunExecuteWorker struct {
	river.WorkerDefaults[RunExecuteArgs]
	Runs *run.Service
}

func (w *RunExecuteWorker) Timeout(*river.Job[RunExecuteArgs]) time.Duration { return 15 * time.Minute }

func (w *RunExecuteWorker) Work(ctx context.Context, job *river.Job[RunExecuteArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	err := w.Runs.Drive(ctx, workspaceID, runID)
	if after, retry := run.RetryAfter(err); retry {
		return river.JobSnooze(after)
	}
	return err
}
