package run

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// PurgeWorkspace is run's share of an account deletion (CORE-007, PDM-006
// §6.1): the artifact rows of the workspace, which is every output a run
// produced plus the download packages built from them. `artifacts` is the one
// table two contexts write, split by kind, and the deletion does not split —
// an account being erased takes both kinds with it.
//
// The run rows themselves stay: a run referencing a version somebody else
// forked is part of that version's provenance (DISC-003), and the privacy the
// owner asked for is delivered by de-identifying the user and workspace rows.
// The objects behind the artifacts are removed by the caller before the
// transaction opens — object storage has no rollback.
//
// Runs on the caller's *gen.Queries and never opens a transaction of its own
// (ADR-034): the account purge is one transaction, all of it or none of it.
func PurgeWorkspace(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	_, err := q.DeleteWorkspaceArtifacts(ctx, workspaceID)
	return err
}
