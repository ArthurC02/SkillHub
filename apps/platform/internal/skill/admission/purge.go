package ingest

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errSourceReadNotInjected = errors.New("ingest: version source read not injected; refusing to purge")

func (s *Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.SourcesInVersions == nil {
		return errSourceReadNotInjected
	}
	q := gen.New(tx)
	sources, err := q.ListWorkspaceSkillSourceIDs(ctx, workspaceID)
	if err != nil {
		return err
	}
	used, err := s.SourcesInVersions(ctx, tx, sources)
	if err != nil {
		return err
	}
	unused := slices.DeleteFunc(sources, func(id pgtype.UUID) bool { return slices.Contains(used, id) })
	if len(unused) == 0 {
		return nil
	}
	_, err = q.DeleteSkillSources(ctx, gen.DeleteSkillSourcesParams{WorkspaceID: workspaceID, SourceIds: unused})
	return err
}
