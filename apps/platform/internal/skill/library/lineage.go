package registry

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type scanAncestor = gen.ListForkedFromVersionsRow

func readScanAncestors(
	ctx context.Context, q *gen.Queries, rows []gen.ListSkillsRow, catalogues []pgtype.UUID,
) (map[pgtype.UUID]scanAncestor, error) {
	var versionIDs []pgtype.UUID
	for _, row := range rows {
		if row.Skill.ForkedFromVersionID.Valid {
			versionIDs = append(versionIDs, row.Skill.ForkedFromVersionID)
		}
	}
	inherited := map[pgtype.UUID]scanAncestor{}
	if len(versionIDs) == 0 {
		return inherited, nil
	}
	forkedFrom, err := q.ListForkedFromVersions(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		i := slices.IndexFunc(forkedFrom, func(v scanAncestor) bool { return v.VersionID == row.Skill.ForkedFromVersionID })
		if i >= 0 && inheritsScan(row, forkedFrom[i], catalogues) {
			inherited[row.Skill.ID] = forkedFrom[i]
		}
	}
	return inherited, nil
}

func inheritsScan(fork gen.ListSkillsRow, ancestor scanAncestor, catalogues []pgtype.UUID) bool {
	return fork.VerifiedAt.Valid && !fork.VerifiedSourceID.Valid &&
		ancestor.SkillID == fork.Skill.ForkedFromSkillID &&
		ancestor.VersionID == fork.Skill.ForkedFromVersionID &&
		slices.Contains(catalogues, ancestor.WorkspaceID) &&
		!ancestor.DeletedAt.Valid && !ancestor.TakedownAt.Valid &&
		ancestor.ContentHash == fork.NewestContentHash
}
