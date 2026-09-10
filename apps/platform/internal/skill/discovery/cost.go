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
	if s.Credit == nil || resp == nil {
		return
	}
	e := credit.CostEvent{
		Kind:  credit.KindSearchEmbedding,
		Model: resp.Model,

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
