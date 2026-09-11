package catalog

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

func (s *Service) recordSearchCost(ctx context.Context, resp *llmclient.EmbedResponse) {
	if resp == nil {
		return
	}
	s.recordCallCost(ctx, credit.KindSearchEmbedding, resp.Model, resp.Usage)
}

func (s *Service) recordCallCost(ctx context.Context, kind, model string, u *llmclient.GatewayUsage) {
	if s.Credit == nil {
		return
	}
	e := credit.CostEvent{
		Kind:           kind,
		Model:          model,
		IdempotencyKey: kind + ":" + uuid.NewString(),
	}
	if u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.CostUSD, u.CostSource)
	} else {
		e.Estimated = true
	}
	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("catalog: model call cost not recorded", "kind", kind, "error", err)
	}
}
