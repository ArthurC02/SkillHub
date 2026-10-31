package eval

import (
	"context"
	"log/slog"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/judging"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

type CreditsForUSD = func(usd float64) (credits int64, ok bool)

func (s *Service) recordEvalCost(ctx context.Context, tx credit.DBTX, kind credit.CostKind, ref evaluationRef, call modelCall) {
	if s.Credit == nil {
		return
	}
	evaluationID, u := ref.evaluationID, call.usage
	e := credit.CostEvent{
		Kind:           kind,
		Model:          call.model,
		PromptVersion:  call.promptVersion,
		WorkspaceID:    ref.workspaceID,
		RefType:        credit.RefRun,
		RefID:          ref.runID,
		IdempotencyKey: string(kind) + ":" + pgconv.UUIDString(evaluationID),
	}
	if u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.ReportedCostUSD())
	} else {
		e.Estimated = true
	}
	if _, _, err := s.Credit.RecordCost(ctx, tx, e); err != nil {

		slog.Warn("eval: model call cost not recorded",
			"kind", kind, "evaluation_id", pgconv.UUIDString(evaluationID), "error", err)
	}
}

func recordScreeningCost(ctx context.Context, recorder CostRecorder, tx credit.DBTX, ref evaluationRef, screening *judging.Screening) {
	if recorder == nil || screening == nil || screening.CostUSD == nil {
		return
	}
	micros, _ := credit.BillableMicros(*screening.CostUSD)
	e := credit.CostEvent{
		Kind:           credit.KindInjectionCheck,
		Model:          screening.Model,
		PromptTokens:   screening.InputTokens,
		UsdMicros:      micros,
		Estimated:      true,
		WorkspaceID:    ref.workspaceID,
		RefType:        credit.RefRun,
		RefID:          ref.runID,
		IdempotencyKey: string(credit.KindInjectionCheck) + ":" + pgconv.UUIDString(ref.evaluationID),
	}
	if _, _, err := recorder.RecordCost(ctx, tx, e); err != nil {
		slog.Warn("eval: injection check cost not recorded",
			"evaluation_id", pgconv.UUIDString(ref.evaluationID), "error", err)
	}
}
