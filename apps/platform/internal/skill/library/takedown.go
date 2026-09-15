package registry

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type TakedownBefore struct {
	WorkspaceID pgtype.UUID
}

func SetTakedown(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason string) (TakedownBefore, error) {
	root, err := loadSkillForOperator(ctx, gen.New(tx), skillID)
	if err != nil {
		return TakedownBefore{}, err
	}
	root.TakeDown(reason)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return TakedownBefore{}, err
	}
	return TakedownBefore{WorkspaceID: root.row.WorkspaceID}, nil
}
