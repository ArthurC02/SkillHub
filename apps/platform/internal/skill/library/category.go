package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Category string

const (
	CategoryDocuments Category = "documents"
	CategoryWriting   Category = "writing"
	CategoryData      Category = "data"
)

func AllCategories() []Category {
	return []Category{CategoryDocuments, CategoryWriting, CategoryData}
}

type CategorySource string

const (
	CategorySourceCurated CategorySource = "curated"
	CategorySourceOwner   CategorySource = "owner"
)

func AllCategorySources() []CategorySource {
	return []CategorySource{CategorySourceCurated, CategorySourceOwner}
}

func (s *Service) SetCategory(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, category *Category) (Skill, error) {
	if s.RefreshListing == nil {
		return Skill{}, errors.New("registry: catalog listing refresh not injected; refusing to write")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root, err := loadSkill(ctx, gen.New(tx), ws.ID, skillID)
	if err != nil {
		return Skill{}, err
	}
	root.Categorize(category)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return Skill{}, err
	}
	if err := s.RefreshListing(ctx, tx, skillID); err != nil {
		return Skill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Skill{}, err
	}
	return root.Skill(), nil
}
