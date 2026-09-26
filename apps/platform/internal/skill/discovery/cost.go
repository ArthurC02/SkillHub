package catalog

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

func (s *Service) recordSearchCost(ctx context.Context, resp *Embeddings) {
	if resp == nil {
		return
	}
	s.recordCallCost(ctx, credit.KindSearchEmbedding, resp.Model, resp.Usage)
}

func (s *Service) recordCallCost(ctx context.Context, kind credit.CostKind, model string, u *ModelUsage) {
	s.recordVersionedCallCost(ctx, kind, model, "", u)
}

func (s *Service) recordVersionedCallCost(ctx context.Context, kind credit.CostKind, model, promptVersion string, u *ModelUsage) {
	if s.Credit == nil {
		return
	}
	e := credit.CostEvent{
		Kind:           kind,
		Model:          model,
		PromptVersion:  promptVersion,
		IdempotencyKey: string(kind) + ":" + uuid.NewString(),
	}
	if u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.reportedCostUSD())
	} else {
		e.Estimated = true
	}
	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("catalog: model call cost not recorded", "kind", kind, "error", err)
	}
}
