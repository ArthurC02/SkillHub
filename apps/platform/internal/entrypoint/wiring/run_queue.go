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

type runJobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (runJobArgs) Kind() string { return "run_execute" }

type cleanupJobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (cleanupJobArgs) Kind() string { return "run_cleanup" }

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

func runJob(work run.RunWork) runJobArgs {
	return runJobArgs{RunID: pgconv.UUIDString(work.RunID), WorkspaceID: pgconv.UUIDString(work.WorkspaceID)}
}
func cleanupJob(work run.RunWork) cleanupJobArgs {
	return cleanupJobArgs{RunID: pgconv.UUIDString(work.RunID), WorkspaceID: pgconv.UUIDString(work.WorkspaceID)}
}
func executeOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: 3}
}
func cleanupOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: 5}
}
