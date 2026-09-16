package registry

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type CurationTier string

const (
	CurationCurated CurationTier = "curated"
	CurationIndexed CurationTier = "indexed"
)

func AllCurationTiers() []CurationTier {
	return []CurationTier{CurationCurated, CurationIndexed}
}

type CurationChange struct {
	WorkspaceID     pgtype.UUID
	Before          CurationTier
	BeforeVersionID pgtype.UUID
	After           CurationTier
	VersionID       pgtype.UUID
}

func SetCuration(
	ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, to CurationTier, catalogues []pgtype.UUID,
) (CurationChange, error) {
	root, err := loadSkillForOperator(ctx, gen.New(tx), skillID)
	if err != nil {
		return CurationChange{}, err
	}
	change := CurationChange{
		WorkspaceID: root.row.WorkspaceID,
		Before:      CurationTier(root.row.CurationTier), BeforeVersionID: root.row.CuratedVersionID,
	}
	root.SetCuration(to, slices.Contains(catalogues, root.row.WorkspaceID))
	if err := SaveSkill(ctx, tx, root); err != nil {
		return CurationChange{}, err
	}
	change.After, change.VersionID = CurationTier(root.row.CurationTier), root.row.CuratedVersionID
	return change, nil
}
