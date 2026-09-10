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
	var source *string
	if category != nil {
		v := categorySourceOwner
		source = &v
	}
	row, err := gen.New(s.Pool).SetSkillCategory(ctx, gen.SetSkillCategoryParams{
		ID: skillID, WorkspaceID: ws.ID, Category: category, CategorySource: source,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, err
	}

	return row, nil
}
