package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const RestrictionLicenseReview = "license-review"

func RestrictionReasons() []string { return []string{RestrictionLicenseReview} }

var ErrEmptyRestriction = errors.New("registry: access restriction must be a non-empty reason code, or nil to lift the hold")

type RestrictionBefore struct {
	WorkspaceID       pgtype.UUID
	AccessRestriction *string
}

func SetAccessRestriction(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason *string) (RestrictionBefore, error) {
	load := loadSkillForOperator
	if reason == nil {
		load = loadSkillForOperatorRelease
	}
	root, err := load(ctx, gen.New(tx), skillID)
	if err != nil {
		return RestrictionBefore{}, err
	}
	before := RestrictionBefore{WorkspaceID: root.row.WorkspaceID, AccessRestriction: root.row.AccessRestriction}
	root.Restrict(reason)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return RestrictionBefore{}, err
	}
	return before, nil
}
