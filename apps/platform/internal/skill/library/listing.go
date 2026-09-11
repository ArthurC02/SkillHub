package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func LiveListingFacts(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) (gen.GetLiveSkillListingFactsRow, bool, error) {
	row, err := gen.New(db).GetLiveSkillListingFacts(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

func LiveSkills(ctx context.Context, db gen.DBTX) ([]gen.ListLiveSkillsForIndexRow, error) {
	return gen.New(db).ListLiveSkillsForIndex(ctx)
}

func LiveSkillIDs(ctx context.Context, db gen.DBTX, skillIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListLiveSkillIDs(ctx, skillIDs)
}
