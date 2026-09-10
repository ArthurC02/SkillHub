package ingest

// What importing and generating cost the platform, written to the spend
// ledger (CRED-005, ADR-068 decision 3: `index_enrich` is the import-time
// enrichment pair, `generate` is one /v1/generate-skill call).
//
// A note on where index enrichment lives, because ADR-032 appendix A and the
// code disagree and the code is right. The appendix puts 索引增強 on
// `catalog` → `credit`; the enrichment calls are in this package
// (enrich.go), because enrichment runs on the import path and the import
// path is ingest's. `ingest` → `credit` is a legal collaboration on that
// same table, so nothing here needs a new permission — the row that named
// catalog was naming the context that owns the search document, not the one
// that pays for it.

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

// CostRecorder is credit.Service narrowed to the one call this context makes
// of it. An interface, not *credit.Service, so a test can read back what was
// written without a ledger behind it.
type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

// recordCost writes one cost_events row for one paid call on the import or
// generation path.
//
// The idempotency key is a fresh uuid per call, and unlike eval's (which is
// the evaluation id) that is the correct answer here rather than a fallback.
// Nothing above these call sites retries them, and the two things that look
// like natural keys are both wrong: the package content hash repeats every
// time the enrichment backfill re-enriches the same document — a second paid
// call that a content-keyed row would silently swallow — and a generation has
// no id at all until the version it produces is written, which is after the
// call it would be keying.
//
// It never returns an error. Every caller is past the point where the money
// was spent, and on the import path enrichment is explicitly allowed to fail
// without failing the import (enrich.go: 「a skill that imported successfully
// must be findable even when the model is not reachable」). A ledger write is
// not a stronger promise than the enrichment it is recording.
func (s *Service) recordCost(ctx context.Context, kind string, workspaceID pgtype.UUID,
	model, promptVersion string, u *llmclient.GatewayUsage) {
	if s.Credit == nil {
		return
	}
	e := credit.CostEvent{
		Kind:           kind,
		Model:          model,
		PromptVersion:  promptVersion,
		WorkspaceID:    workspaceID,
		IdempotencyKey: kind + ":" + uuid.NewString(),
	}
	if u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.CostUSD, u.CostSource)
	} else {
		e.Estimated = true
	}
	// s.Pool rather than a transaction: enrichment deliberately runs outside
	// the import transaction (holding one open across two network round-trips
	// pins a connection for the length of a model call), and generation's
	// transaction does not exist until importZip opens it, after the call.
	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("ingest: model call cost not recorded", "kind", kind, "error", err)
	}
}
