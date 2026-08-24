package analytics

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeWorkspace is analytics' share of an account deletion (CORE-007,
// PDM-006 §6.1). Its answer is de-identification rather than deletion (ADR-029
// 決策 5): the funnel is compared quarter over quarter, so removing a departed
// user's events would rewrite last quarter's numbers, and a beta tester's words
// are the evidence a scope review rests on. What goes is the link to the person.
//
// That distinction is analytics' domain knowledge, which is why the statements
// live here instead of in identity's purge transaction (ADR-034). The cascade
// does not cover it either: the purge anonymises the workspace row rather than
// deleting it, so 0029's ON DELETE SET NULL never fires.
//
// It runs on the caller's transaction and never opens one of its
// own. That is the contract — CORE-007 promises one transaction that clears
// every context or none, and a step that began its own would turn one guarantee
// into six that can each fail alone.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	if _, err := q.DetachWorkspaceAnalytics(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.DetachWorkspaceFeedback(ctx, workspaceID)
	return err
}
