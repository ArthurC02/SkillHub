package registry

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// testProjection wires the search projection writes the way apiserver.NewApp
// does. It repeats catalog's one-line wrappers rather than calling them because
// catalog imports this package (DDD-020), so a test binary that imported catalog
// back would be an import cycle — which is the whole reason these writes are
// injected instead of imported (ADR-034).
func testProjection(s *Service) *Service {
	s.IndexSkill = func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error {
		return gen.New(tx).UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
			SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
			Name: projection.Name, Summary: projection.Summary,
		})
	}
	s.RemoveFromIndex = func(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID) error {
		return gen.New(tx).DeleteSearchDocument(ctx, skillID)
	}
	return s
}

// A service assembled without the projection writes must refuse, not proceed
// and skip the index: a fork nobody can search for and a takedown still listed
// both look exactly like success from the caller's side. No pool is set, so any
// path that reached the transaction would panic instead of returning — which is
// also how this test proves the check comes first.
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
