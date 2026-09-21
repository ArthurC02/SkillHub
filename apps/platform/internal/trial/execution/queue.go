package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type RunWork struct {
	RunID       pgtype.UUID
	WorkspaceID pgtype.UUID
}

type RunQueue interface {
	Drive(context.Context, RunWork) (added bool, err error)
	DriveInTx(context.Context, pgx.Tx, RunWork) error
	Clean(context.Context, RunWork) error
	CleanInTx(context.Context, pgx.Tx, RunWork) error
}
