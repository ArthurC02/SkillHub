package testlab

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (*Service) WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceDatasetObjectKeys(ctx, workspaceID)
}

func (s *Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.ClearSightings == nil {
		return errPersistenceNotConfigured
	}
	q := gen.New(tx)
	ids, err := q.DeleteWorkspaceDatasets(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.ClearSightings(ctx, tx, ids); err != nil {
		return err
	}
	_, err = q.DeleteWorkspaceTestCases(ctx, workspaceID)
	return err
}
