package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var errWorkspaceCreatedAtTransactionRequired = errors.New("identity: workspace-created-at transaction is required")

// ReadWorkspaceCreatedAt is the transaction-capable Identity owner read used by
// create-run. The caller owns the transaction; Identity owns its sqlc query.
func ReadWorkspaceCreatedAt(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) (time.Time, error) {
	if tx == nil {
		return time.Time{}, errWorkspaceCreatedAtTransactionRequired
	}
	created, err := gen.New(tx).GetWorkspaceCreatedAt(ctx, workspaceID)
	if err != nil {
		return time.Time{}, err
	}
	return created.Time, nil
}

// WorkspaceCreatedAt is the pool-backed read used by quota display paths.
func (s *Service) WorkspaceCreatedAt(ctx context.Context, workspaceID pgtype.UUID) (time.Time, error) {
	created, err := gen.New(s.Pool).GetWorkspaceCreatedAt(ctx, workspaceID)
	if err != nil {
		return time.Time{}, err
	}
	return created.Time, nil
}
