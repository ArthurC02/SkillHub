package packaging

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeWorkspace removes this workspace's download-package rows inside the
// caller's account-purge transaction. Run outputs share the physical artifacts
// table but remain run's responsibility; the kind predicate enforces the split.

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.DeleteWorkspaceDownloadArtifacts(ctx, workspaceID)
	return err
}

// WorkspaceObjectKeys names only this context's download-package objects.
// Identity removes the bytes before opening the database transaction because
// object storage cannot roll back.
func (s *Service) WorkspaceObjectKeys(ctx context.Context, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(s.Pool).ListWorkspaceDownloadArtifactObjectKeys(ctx, workspaceID)
}
