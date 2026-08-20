package testlab

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// PurgeWorkspace is the test lab's share of an account deletion (CORE-007,
// PDM-006 §6.1): uploaded dataset files are private content and go for real.
// Test case snapshots a run points at are not deleted here — they are frozen
// (iron rule 4) and belong to the run's record of what it was given.
//
// The objects behind these rows are removed by the caller before the
// transaction opens, on purpose: object storage has no rollback, so the two
// halves cannot commit together and the only order that never leaves a user's
// file alive with nothing in the database naming it is objects first.
//
// Runs on the caller's *gen.Queries and never opens a transaction of its own
// (ADR-034): the account purge is one transaction, all of it or none of it.
func PurgeWorkspace(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	_, err := q.DeleteWorkspaceDatasets(ctx, workspaceID)
	return err
}
