package registry

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var ErrEmptyRestriction = errors.New("registry: access restriction must be a non-empty reason code, or nil to lift the hold")

type RestrictionBefore struct {
	WorkspaceID       pgtype.UUID
	AccessRestriction *string
}

func SetAccessRestriction(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason *string) (RestrictionBefore, error) {
	if reason != nil && strings.TrimSpace(*reason) == "" {
		return RestrictionBefore{}, ErrEmptyRestriction
	}
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RestrictionBefore{}, ErrNotFound
	}
	if err != nil {
		return RestrictionBefore{}, err
	}
	if err := q.SetSkillAccessRestriction(ctx, gen.SetSkillAccessRestrictionParams{
		ID: skillID, AccessRestriction: reason,
	}); err != nil {
		return RestrictionBefore{}, err
	}
	return RestrictionBefore{
		WorkspaceID: before.WorkspaceID, AccessRestriction: before.AccessRestriction,
	}, nil
}
