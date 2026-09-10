package packaging

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	// Children before the parent row, to satisfy the foreign keys.
	q := gen.New(tx)
	if _, err := q.DeleteWorkspaceDownloadRecords(ctx, workspaceID); err != nil {
		return err
	}
	if _, err := q.DeleteWorkspaceDownloadArtifactDetails(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.DeleteWorkspaceDownloadArtifacts(ctx, workspaceID)
	return err
}

func (*Service) WorkspaceObjectKeys(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(db).ListWorkspaceDownloadArtifactObjectKeys(ctx, workspaceID)
}
