package eval

// The queue side. Go is the only queue consumer (iron rule 7): this job calls
// services/llm over internal HTTP, and Python never subscribes to anything.

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// JobArgs is the queue payload for evaluating one run. Identifiers only, like
// run.JobArgs: everything else is read from the database under the workspace it
// names, so a tampered payload cannot widen scope.
type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

// Kind is the durable job name. Renaming it strands jobs already in the queue.
func (JobArgs) Kind() string { return "evaluate_run" }

// liveJobStates are the states in which a job still counts for uniqueness. The
// default set also includes `completed`, which would make a run un-re-evaluatable
// after the first pass — and re-evaluation is a thing this platform does
// deliberately (ADR-026).
var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
	rivertype.JobStateRetryable,
}

// InsertOpts keeps one live evaluation per run. Exported because internal/run
// enqueues this job from the terminal transition and must use the same options:
// an insert without them carries no unique key, and a redelivered transition
// would pay for a second judge call to reach the same verdict.
//
// Two attempts, not River's twenty-five: a judgement that failed because the
// gateway was down is worth one retry, and one that failed because the evidence
// is unreadable will fail identically forever. `evaluations.status = failed` is a
// visible outcome, so exhausting the retries is not a silent loss.
func InsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
		MaxAttempts: 2,
	}
}

// Worker evaluates one finished run.
type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err // malformed payload: nothing to look up, let River discard it
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}
	return w.Svc.Evaluate(ctx, workspaceID, runID)
}
