package run

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeWorkspace is run's share of an account deletion (CORE-007, PDM-006
// §6.1): the run-output artifact rows of the workspace. Download packages share
// the physical `artifacts` table but belong to packaging and are purged there.
//
// The run rows themselves stay: a run referencing a version somebody else
// forked is part of that version's provenance (DISC-003), and the privacy the
// owner asked for is delivered by de-identifying the user and workspace rows.
// The objects behind the artifacts are removed by the caller before the
// transaction opens — object storage has no rollback.
//
// Runs on the caller's transaction and never opens one of its own
// (ADR-034): the account purge is one transaction, all of it or none of it.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.DeleteWorkspaceRunArtifacts(ctx, workspaceID)
	return err
}

// WorkspaceObjectKeys names only this context's run-output objects. Identity
// calls it before opening the account-purge transaction because object storage
// cannot participate in that transaction.
func (s *Service) WorkspaceObjectKeys(ctx context.Context, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(s.Pool).ListWorkspaceRunArtifactObjectKeys(ctx, workspaceID)
}
