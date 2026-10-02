package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type runQueue struct{ client *river.Client[pgx.Tx] }

type RunExecuteArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (RunExecuteArgs) Kind() string { return "run_execute" }

type RunCleanupArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (RunCleanupArgs) Kind() string { return "run_cleanup" }

const (
	runExecuteMaxAttempts = 3
	runCleanupMaxAttempts = 5
)

var liveRunJobStates = []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable}

func NewRunQueue(client *river.Client[pgx.Tx]) run.RunQueue {
	if client == nil {
		return nil
	}
	return &runQueue{client: client}
}

func (q *runQueue) Drive(ctx context.Context, work run.RunWork) (bool, error) {
	result, err := q.client.Insert(ctx, runJob(work), executeOptions())
	if err != nil {
		return false, err
	}
	return !result.UniqueSkippedAsDuplicate, nil
}

func (q *runQueue) DriveInTx(ctx context.Context, tx pgx.Tx, work run.RunWork) error {
	_, err := q.client.InsertTx(ctx, tx, runJob(work), executeOptions())
	return err
}

func (q *runQueue) Clean(ctx context.Context, work run.RunWork) error {
	_, err := q.client.Insert(ctx, cleanupJob(work), cleanupOptions())
	return err
}

func (q *runQueue) CleanInTx(ctx context.Context, tx pgx.Tx, work run.RunWork) error {
	_, err := q.client.InsertTx(ctx, tx, cleanupJob(work), cleanupOptions())
	return err
}

func runJob(work run.RunWork) RunExecuteArgs {
	return RunExecuteArgs{RunID: pgconv.UUIDString(work.RunID), WorkspaceID: pgconv.UUIDString(work.WorkspaceID)}
}
func cleanupJob(work run.RunWork) RunCleanupArgs {
	return RunCleanupArgs{RunID: pgconv.UUIDString(work.RunID), WorkspaceID: pgconv.UUIDString(work.WorkspaceID)}
}
func executeOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: runExecuteMaxAttempts, Queue: QueueRuns}
}
func cleanupOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: runCleanupMaxAttempts, Queue: QueueRuns}
}
