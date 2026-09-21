package run

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type RunQueue interface {
	Drive(ctx context.Context, run gen.Run) (added bool, err error)
	DriveInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error
	Clean(ctx context.Context, run gen.Run) error
	CleanInTx(ctx context.Context, tx pgx.Tx, run gen.Run) error
}
