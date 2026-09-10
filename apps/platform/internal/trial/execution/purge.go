package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.DeleteWorkspaceRunArtifacts(ctx, workspaceID)
	return err
}

func (*Service) WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceRunArtifactObjectKeys(ctx, workspaceID)
}

func (*Service) PurgeQuiescent(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
	return gen.New(db).AccountPurgeReady(ctx, workspaceID)
}
