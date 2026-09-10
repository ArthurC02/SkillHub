package analytics

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	if _, err := q.DetachWorkspaceAnalytics(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.DetachWorkspaceFeedback(ctx, workspaceID)
	return err
}
