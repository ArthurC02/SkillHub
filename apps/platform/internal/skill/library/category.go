package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const categorySourceOwner = "owner"

func (s *Service) SetCategory(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, category *string) (gen.Skill, error) {
	if s.RefreshListing == nil {
		return gen.Skill{}, errors.New("registry: catalog listing refresh not injected; refusing to write")
	}
	var source *string
	if category != nil {
		v := categorySourceOwner
		source = &v
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := gen.New(tx).SetSkillCategory(ctx, gen.SetSkillCategoryParams{
		ID: skillID, WorkspaceID: ws.ID, Category: category, CategorySource: source,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, err
	}
	if err := s.RefreshListing(ctx, tx, skillID); err != nil {
		return gen.Skill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Skill{}, err
	}
	return row, nil
}
