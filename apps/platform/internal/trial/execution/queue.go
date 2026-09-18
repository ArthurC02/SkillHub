package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type RunQueue interface {
	Drive(ctx context.Context, run gen.Run) (added bool, err error)
	DriveInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error
	Clean(ctx context.Context, run gen.Run) error
	CleanInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error
}

// A nil *river.Client inside a non-nil interface passes every `Queue != nil`
// guard and panics on the first call.
func NewRunQueue(client *river.Client[pgx.Tx]) RunQueue {
	if client == nil {
		return nil
	}
	return &riverQueue{client: client}
}

type riverQueue struct {
	client *river.Client[pgx.Tx]
}

func (q *riverQueue) Drive(ctx context.Context, run gen.Run) (bool, error) {
	res, err := q.client.Insert(ctx, executeArgs(run), executeInsertOpts())
	if err != nil {
		return false, err
	}
	return !res.UniqueSkippedAsDuplicate, nil
}

func (q *riverQueue) DriveInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error {
	_, err := q.client.InsertTx(ctx, tx, executeArgs(run), executeInsertOpts())
	return err
}

func (q *riverQueue) Clean(ctx context.Context, run gen.Run) error {
	_, err := q.client.Insert(ctx, cleanupArgs(run), cleanupInsertOpts())
	return err
}

func (q *riverQueue) CleanInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error {
	_, err := q.client.InsertTx(ctx, tx, cleanupArgs(run), cleanupInsertOpts())
	return err
}

func executeArgs(run gen.Run) JobArgs {
	return JobArgs{RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID)}
}

func cleanupArgs(run gen.Run) CleanupArgs {
	return CleanupArgs{RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID)}
}
