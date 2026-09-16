package registry

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type RedistributionBefore struct {
	WorkspaceID    pgtype.UUID
	Redistribution string
	Verified       LicenseClaim
}

func SetRedistribution(
	ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, value string, claim LicenseClaim,
) (RedistributionBefore, error) {
	q := gen.New(tx)
	root, err := loadSkillForOperator(ctx, q, skillID)
	if err != nil {
		return RedistributionBefore{}, err
	}
	before := RedistributionBefore{WorkspaceID: root.row.WorkspaceID, Redistribution: root.row.Redistribution}
	to := Redistribution(value)
	root.SetRedistribution(to, claim)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return RedistributionBefore{}, err
	}
	if to == RedistributionAllowed {
		before.Verified = root.NewestLicense()
	}
	return before, nil
}
