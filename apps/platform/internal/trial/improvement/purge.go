package eval

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
)

func PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if _, err := gen.New(tx).ClearWorkspaceEvaluationComments(ctx, workspaceID); err != nil {
		return err
	}
	return graders.PurgeWorkspace(ctx, tx, workspaceID)
}
