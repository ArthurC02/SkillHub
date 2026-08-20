package run

// Run's two touch points on the PDM-010 allowance. The allowance itself — the
// limits, the counting query and the refusal rule — is internal/policy's
// (Policy & Usage, ADR-032 §1); DDD-014 moved it there and left these behind.
//
// What stayed, and why:
//
//	requireQuota  the enforcement *point*. ADR-028 決策 2 puts it inside the
//	              create-run transaction under requireRunSlot's advisory lock, and
//	              that is a Run Orchestration fact — policy answers the question,
//	              gate B is where the question gets asked and the answer acted on.
//	QuotaFor      the read path, plus the one limit policy does not own: the
//	              workspace concurrency ceiling is run's invariant, and the screen
//	              wants all four numbers in one place.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/policy"
)

// requireQuota is gate B's allowance condition (PDM-010, ADR-028 決策 2).
//
// q must be the create-run transaction's handle, and requireRunSlot must already
// have run on it — see policy.EnforceQuota for what that lock does and does not
// buy. The refusal is counted and labelled here, with the reason string policy
// returned, so the Prometheus label and the audit reason stay one string.
func (s *Service) requireQuota(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	reason, err := policy.EnforceQuota(ctx, q, s.Quota, workspaceID)
	if err != nil {
		return refused(reason, err)
	}
	return nil
}

// QuotaFor is the read path behind GET /me/quota and the pre-run summary's quota
// block. ok is false when this deployment enforces no allowance, and the caller
// must then show nothing rather than zeroes.
func (s *Service) QuotaFor(ctx context.Context, workspaceID pgtype.UUID) (policy.QuotaState, bool, error) {
	if !s.Quota.Enforced() {
		return policy.QuotaState{}, false, nil
	}
	state, err := policy.Usage(ctx, s.queries(), s.Quota, workspaceID, time.Now())
	if err != nil {
		return policy.QuotaState{}, false, err
	}
	// The fourth limit, added on the way to the screen: PDM-005 §5.2's concurrency
	// ceiling is this context's, not Policy & Usage's, and a user discovering it by
	// hitting it is the reason all four are reported together.
	state.Limits.Concurrent = MaxConcurrentRunsPerWorkspace
	return state, true, nil
}
