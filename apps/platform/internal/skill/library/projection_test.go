package registry

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func testProjection(s *Service) *Service {
	s.IndexSkill = func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error {
		return gen.New(tx).UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
			SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
			Name: projection.Name, Summary: projection.Summary,
		})
	}
	s.RemoveFromIndex = func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) error {
		return gen.New(tx).DeleteSearchDocument(ctx, gen.DeleteSearchDocumentParams{
			SkillID: skillID, WorkspaceID: workspaceID,
		})
	}
	return s
}

func TestWritesRefuseWithoutTheProjectionWrites(t *testing.T) {
	ctx := context.Background()
	ws := identity.Workspace{}
	for name, call := range map[string]func(*Service) error{
		"Fork": func(s *Service) error {
			_, _, err := s.Fork(ctx, ws, pgtype.UUID{})
			return err
		},
		"Delete": func(s *Service) error {
			_, err := s.Delete(ctx, ws, pgtype.UUID{})
			return err
		},
		"Takedown": func(s *Service) error {
			_, err := s.Takedown(ctx, ws, pgtype.UUID{}, "reason")
			return err
		},
	} {
		if err := call(&Service{}); err == nil {
			t.Errorf("%s succeeded without the search projection writes injected", name)
		}
	}
}
