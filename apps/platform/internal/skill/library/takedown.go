package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type TakedownBefore struct {
	WorkspaceID pgtype.UUID
}

func SetTakedown(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason string) (TakedownBefore, error) {
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TakedownBefore{}, ErrNotFound
	}
	if err != nil {
		return TakedownBefore{}, err
	}
	if before.TakedownAt.Valid {
		return TakedownBefore{}, ErrAlreadyTakenDown
	}
	if err := q.SetSkillTakedown(ctx, gen.SetSkillTakedownParams{
		ID: skillID, TakedownReason: &reason,
	}); err != nil {
		return TakedownBefore{}, err
	}
	return TakedownBefore{WorkspaceID: before.WorkspaceID}, nil
}
