package testlab

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// WorkspaceObjectKeys names this context's private uploaded content in one
// workspace, so
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
func (s *Service) WorkspaceObjectKeys(ctx context.Context, workspaceID pgtype.UUID) ([]string, error) {
	return gen.New(s.Pool).ListWorkspaceDatasetObjectKeys(ctx, workspaceID)
}

// PurgeWorkspace is the test lab's share of an account deletion (CORE-007,
// PDM-006 §6.1): uploaded dataset files are private content and go for real,
// and so are the test cases, which hold the user's own words (`user_prompt`,
// 02:TEST-001). Until 05 R-29 was signed on 2026-08-30 the second half did not
// exist — this table was the last class of user-submitted free text in the
// repository with no deletion path at all, account deletion included, while
// datasets, feedback and generated task descriptions each had one.
//
// Test case snapshots a run points at are not deleted here — they are frozen
// (iron rule 4) and belong to the run's record of what it was given. That is
// also why the delete carries NOT EXISTS rather than a filter anyone could
// tighten later: the snapshot's FK would refuse the row anyway, and a snapshot
// keeps its own copy of the prompt, which outlives this purge attached to a
// workspace the same transaction de-identifies.
//
// The objects behind these rows are removed by the caller before the
// transaction opens, on purpose: object storage has no rollback, so the two
// halves cannot commit together and the only order that never leaves a user's
// file alive with nothing in the database naming it is objects first.
//
// Runs on the caller's *gen.Queries and never opens a transaction of its own
// (ADR-034): the account purge is one transaction, all of it or none of it.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	if _, err := q.DeleteWorkspaceDatasets(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.DeleteWorkspaceTestCases(ctx, workspaceID)
	return err
}
