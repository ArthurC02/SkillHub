package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type RunExecuteWorker struct {
	river.WorkerDefaults[wiring.RunExecuteArgs]
	Runs *run.Service
}

const runExecuteTimeout = 15 * time.Minute

func (w *RunExecuteWorker) Timeout(*river.Job[wiring.RunExecuteArgs]) time.Duration {
	return runExecuteTimeout
}

func (w *RunExecuteWorker) Work(ctx context.Context, job *river.Job[wiring.RunExecuteArgs]) error {
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
