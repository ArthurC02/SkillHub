package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type CatalogReads struct {
	db         gen.DBTX
	workspaces func(context.Context, gen.DBTX) ([]pgtype.UUID, error)
}

func (s *Service) CatalogReads() CatalogReads {
	return CatalogReads{db: s.Pool, workspaces: s.CatalogWorkspaces}
}

func (s *Service) CatalogSkill(ctx context.Context, skillID pgtype.UUID) (Skill, bool, error) {
	return s.CatalogReads().Skill(ctx, skillID)
}

func (r CatalogReads) workspaceIDs(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error) {
	if r.workspaces == nil {
		return nil, errors.New("registry: catalog workspace read not injected")
	}
	return r.workspaces(ctx, db)
}

func (r CatalogReads) skillIn(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) (gen.Skill, error) {
	catalogs, err := r.workspaceIDs(ctx, db)
	if err != nil {
		return gen.Skill{}, err
	}
	return gen.New(db).GetCatalogSkill(ctx, gen.GetCatalogSkillParams{ID: skillID, CatalogWorkspaceIds: catalogs})
}

func (r CatalogReads) Skill(ctx context.Context, skillID pgtype.UUID) (Skill, bool, error) {
	row, err := r.skillIn(ctx, r.db, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	skill, err := withLineage(ctx, r.db, skillDTO(row))
	return skill, err == nil, err
}

func (r CatalogReads) Ancestor(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillRef, bool, error) {
	catalogs, err := r.workspaceIDs(ctx, r.db)
	if err != nil {
		return SkillRef{}, false, err
	}
	row, err := gen.New(r.db).GetCuratedAncestor(ctx, gen.GetCuratedAncestorParams{
		SkillID: skillID, WorkspaceID: workspaceID, CatalogWorkspaceIds: catalogs,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SkillRef{}, false, nil
	}
	if err != nil {
		return SkillRef{}, false, err
	}
	return SkillRef{WorkspaceID: row.WorkspaceID, SkillID: row.ID}, true, nil
}
