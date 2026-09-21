package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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

func (q *runQueue) Drive(ctx context.Context, r gen.Run) (bool, error) {
	result, err := q.client.Insert(ctx, runJob(r), executeOptions())
	if err != nil {
		return false, err
	}
	return !result.UniqueSkippedAsDuplicate, nil
}

func (q *runQueue) DriveInTx(ctx context.Context, tx pgx.Tx, r gen.Run) error {
	_, err := q.client.InsertTx(ctx, tx, runJob(r), executeOptions())
	return err
}

func (q *runQueue) Clean(ctx context.Context, r gen.Run) error {
	_, err := q.client.Insert(ctx, cleanupJob(r), cleanupOptions())
	return err
}

func (q *runQueue) CleanInTx(ctx context.Context, tx pgx.Tx, r gen.Run) error {
	_, err := q.client.InsertTx(ctx, tx, cleanupJob(r), cleanupOptions())
	return err
}

func runJob(r gen.Run) runJobArgs {
	return runJobArgs{RunID: pgconv.UUIDString(r.ID), WorkspaceID: pgconv.UUIDString(r.WorkspaceID)}
}
func cleanupJob(r gen.Run) cleanupJobArgs {
	return cleanupJobArgs{RunID: pgconv.UUIDString(r.ID), WorkspaceID: pgconv.UUIDString(r.WorkspaceID)}
}
func executeOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: 3}
}
func cleanupOptions() *river.InsertOpts {
	return &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveRunJobStates}, MaxAttempts: 5}
}
