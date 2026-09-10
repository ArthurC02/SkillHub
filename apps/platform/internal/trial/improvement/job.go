package eval

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (JobArgs) Kind() string { return "evaluate_run" }

var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
	rivertype.JobStateRetryable,
}

func InsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
		MaxAttempts: 2,
	}
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	if job.Attempt > 1 {
		return w.Svc.recoverAttempt(ctx, workspaceID, runID)
	}
	return w.Svc.Evaluate(ctx, workspaceID, runID)
}
