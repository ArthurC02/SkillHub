package catalog

// What catalogue search costs the platform, written to the spend ledger
// (CRED-005, ADR-068 decision 3).
//
// Catalog is the one context that only ever writes here. It does not ask
// credit whether it may proceed and it never debits an account: ADR-032
// appendix A's row says so — 「搜尋 embedding、索引增強寫成本事件；MVP 不對
// 這兩者扣點」 — because search is free to the user in MVP while still
// costing the platform an embedding call. The rows exist so that the p95 the
// start gate is derived from is computed over the platform's whole spend and
// not only over the part somebody was charged for.

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

// CostRecorder is credit.Service narrowed to the single call catalog makes of
// it. An interface rather than *credit.Service so that a test can see what was
// written without standing up a ledger, and so this package states its whole
// dependency on credit in three lines.
type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

// recordSearchCost writes one cost_events row for one query-embedding call.
//
// Three things about it are deliberate:
//
// Anonymous. Public search has no session middleware at all (DISC-010), so
// there is no workspace and no user to attribute this to, and both columns are
// nullable precisely for this row (migration 0060's own comment: 「an anonymous
// catalog search still costs an embedding call」). Attributing it to something
// would be inventing an owner.
//
// No query text, and that is structural rather than careful: [credit.CostEvent]
// has no field a query could go in. ADR-029's rule for analytics events —
// what a search cost is the platform's fact, what somebody searched for is
// not — is the same rule here.
//
// Never fails the search. The money is already spent by the time this runs;
// turning a ledger write into a 500 would charge the platform for the call
// AND lose the result it paid for. A dropped row is a hole in the statistics,
// which is why it is logged loudly rather than swallowed.
func (s *Service) recordSearchCost(ctx context.Context, resp *llmclient.EmbedResponse) {
	if s.Credit == nil || resp == nil {
		return
	}
	e := credit.CostEvent{
		Kind:  credit.KindSearchEmbedding,
		Model: resp.Model,
		// A fresh key per call, and it is not standing in for a real replay
		// key: nothing upstream of embedQuery retries — this is a synchronous
		// handler, not a River job — so there is no second delivery to
		// collapse. It exists because cost_events.idempotency_key is NOT NULL
		// and unique, and inventing a deterministic key out of the query text
		// would put the query in the row this file exists to keep it out of.
		IdempotencyKey: "search_embedding:" + uuid.NewString(),
	}
	if u := resp.Usage; u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.CostUSD, u.CostSource)
	} else {
		e.Estimated = true
	}
	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("catalog: search embedding cost not recorded", "error", err)
	}
}
