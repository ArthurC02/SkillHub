package eval

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

type CreditsForUSD = func(usd float64) (credits int64, ok bool)

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

		slog.Warn("eval: model call cost not recorded",
			"kind", kind, "evaluation_id", pgconv.UUIDString(evaluationID), "error", err)
	}
}
