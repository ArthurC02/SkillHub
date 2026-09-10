package ingest

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

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

	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("ingest: model call cost not recorded", "kind", kind, "error", err)
	}
}
