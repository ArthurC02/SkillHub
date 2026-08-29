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
//
// Three statements, and the order is 0027's foreign keys rather than a
// preference: download_records hangs off download_artifacts, which hangs off
// artifacts, both composite and neither with ON DELETE. Until 2026-08-29 this
// function was the third statement alone, so it raised 23503 on every workspace
// that had ever produced a package — and because the whole account deletion is
// one transaction (ADR-034), that one error rolled back every other context's
// step too and the account stayed on the worklist forever, silently: the user
// had already been told DELETE /me succeeded, and PurgeExpiredAccounts' error
// only reaches a log line.
//
// It survived review because the integration test's fixture inserted the
// `artifacts` row and neither detail row, so the test built a world in which
// this function was complete. The fixture is the fix (see
// governance_integration_test.go); this is the code it now catches.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
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

// WorkspaceObjectKeys names only this context's download-package objects.
// Identity removes the bytes before opening the database transaction because
// object storage cannot roll back.
func (s *Service) WorkspaceObjectKeys(ctx context.Context, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(s.Pool).ListWorkspaceDownloadArtifactObjectKeys(ctx, workspaceID)
}
