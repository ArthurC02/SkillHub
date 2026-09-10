package eval

// What judging and advising cost the platform, written to the spend ledger
// (CRED-005, ADR-068 decision 3: `review` is the judge call, `suggestion` is
// suggest-improvements).
//
// This is not a second copy of evaluation_model_usage. That table is the bill
// for one evaluation and is read by this context; cost_events is the
// platform's whole spend across every context, and it is what the rolling
// window behind the start gate is computed over (ADR-068 decision 8). One of
// them answers 「這次評估花了多少」, the other answers 「平台花了多少」, and
// merging them would mean the gate's p95 could only ever see the calls this
// package happened to make.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// CostRecorder is credit.Service narrowed to the one call this context makes
// of it. An interface, not *credit.Service, so a test can read back what was
// written without a ledger behind it.
type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

// CreditsForUSD is the other half of this context's relationship with credit,
// and it points the other way: recording spend is a write, showing spend is a
// conversion. It is credit.Service.CreditsForUSD, injected as a plain func for
// the reason [CostRecorder] is a narrow interface — what eval needs of credit
// is two operations, not a package.
//
// ok=false means the amount has no credit representation; every caller renders
// absence rather than zero, which is the same disposition they already had for
// a cost the gateway never reported.
type CreditsForUSD = func(usd float64) (credits int64, ok bool)

// recordEvalCost writes one cost_events row for one paid call.
//
// The evaluation id is the idempotency key and it is a real one, not a
// placeholder: a redelivered job that judges the same evaluation again writes
// the same key and the second write collapses onto the first. That is the
// same guarantee evaluation_model_usage already gets from its
// (evaluation_id, operation) conflict clause — stated here in this table's own
// vocabulary rather than inherited from it.
//
// ref is the Run, not the evaluation: ADR-068 decision 3's ref_type is a
// closed set of three (creation session / run / skill version) and an
// evaluation is not one of them. The run is what the evaluation is about and
// is the id that leads back to it.
//
// user_id stays null. This path runs from a run-completion consumer, not from
// a request, and no user id reaches it — nor should one be conjured from the
// workspace here: which account a judgement is billed to is a question for the
// debit half of this collaboration, and this half only records spend.
func (s *Service) recordEvalCost(ctx context.Context, tx credit.DBTX, kind string,
	evaluationID, workspaceID, runID pgtype.UUID, model, promptVersion string, u *llmclient.GatewayUsage) {
	if s.Credit == nil {
		return
	}
	e := credit.CostEvent{
		Kind:           kind,
		Model:          model,
		PromptVersion:  promptVersion,
		WorkspaceID:    workspaceID,
		RefType:        credit.RefRun,
		RefID:          runID,
		IdempotencyKey: kind + ":" + pgconv.UUIDString(evaluationID),
	}
	if u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.CostUSD, u.CostSource)
	} else {
		e.Estimated = true
	}
	if _, _, err := s.Credit.RecordCost(ctx, tx, e); err != nil {
		// Logged, never returned. The judge leg's caller is inside the
		// transaction that commits the verdict, and a verdict that was paid
		// for and then rolled back because its spend row would not write is
		// the worse of the two failures — the money is gone either way, and
		// only one of the two outcomes also loses the evaluation.
		slog.Warn("eval: model call cost not recorded",
			"kind", kind, "evaluation_id", pgconv.UUIDString(evaluationID), "error", err)
	}
}
