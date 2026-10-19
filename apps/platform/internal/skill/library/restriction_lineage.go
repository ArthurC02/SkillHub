package registry

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func inheritedRestrictions(ctx context.Context, db gen.DBTX, skillIDs []pgtype.UUID) (map[pgtype.UUID]string, error) {
	inherited := map[pgtype.UUID]string{}
	if len(skillIDs) == 0 {
		return inherited, nil
	}
	rows, err := gen.New(db).ListLineageRestrictions(ctx, skillIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		inherited[row.SkillID] = row.Reason
	}
	return inherited, nil
}

func withLineage(ctx context.Context, db gen.DBTX, skill Skill) (Skill, error) {
	if !skill.ForkedFromSkillID.Valid || RestrictionFrom(skill.AccessRestriction).InEffect() {
		return skill, nil
	}
	inherited, err := inheritedRestrictions(ctx, db, []pgtype.UUID{skill.ID})
	skill.InheritedRestriction = inherited[skill.ID]
	return skill, err
}
