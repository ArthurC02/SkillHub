package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

type CostRecorder interface {
	RecordCost(ctx context.Context, tx credit.DBTX, e credit.CostEvent) (id string, existed bool, err error)
}

const costRecordTimeout = 5 * time.Second

type modelCall struct {
	model         string
	promptVersion string
	usage         *ModelUsage
}

func (s *Service) recordCost(ctx context.Context, kind credit.CostKind, workspaceID pgtype.UUID, call modelCall) {
	if s.Credit == nil {
		return
	}
	e := credit.CostEvent{
		Kind:           kind,
		Model:          call.model,
		PromptVersion:  call.promptVersion,
		WorkspaceID:    workspaceID,
		IdempotencyKey: string(kind) + ":" + uuid.NewString(),
	}
	if u := call.usage; u != nil {
		e.PromptTokens, e.CompletionTokens = u.PromptTokens, u.CompletionTokens
		e.UsdMicros, e.Estimated = credit.UsageCost(u.ReportedCostUSD())
	} else {
		e.Estimated = true
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), costRecordTimeout)
	defer cancel()
	if _, _, err := s.Credit.RecordCost(ctx, s.Pool, e); err != nil {
		slog.Warn("ingest: model call cost not recorded", "kind", kind, "error", err)
	}
}
