package testlab

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// WorkspaceObjectKeys names the private uploaded content of one workspace, so
// identity's account purge can delete the objects before it opens the transaction
// that deletes the rows (CORE-007, PDM-006 §6.1).
//
// Separate from [PurgeWorkspace] rather than folded into it, because the two
// halves deliberately do not run together: object storage has no rollback, so the
// purge removes objects first and rows second, and a function answering from
// inside that transaction would answer after the only moment its caller can act
// on the answer. Injected rather than imported for the same reason
// [PurgeWorkspace] is (ADR-034) - and identity -> testlab is denied by ADR-032
// appendix A besides, so an import was never available here.
//
// One wart, named rather than hidden: the query behind this unions `datasets`
// with `artifacts`, so it also answers for run's and packaging's rows.
// db/query-owners.yaml declares this package its owner, which is why the function
// lives here; splitting it by owner means splitting the query, and db/queries is
// not this change's to touch (ADR-035 C 組).
func WorkspaceObjectKeys(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) ([]string, error) {
	return q.ListWorkspaceObjectKeys(ctx, workspaceID)
}

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
