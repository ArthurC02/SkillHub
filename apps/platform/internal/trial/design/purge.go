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

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	if _, err := q.DeleteWorkspaceDatasets(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.DeleteWorkspaceTestCases(ctx, workspaceID)
	return err
}
