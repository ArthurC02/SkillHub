package wiring

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func NewAgentRunner(
	pool *pgxpool.Pool, llm *llmclient.Client, gateway *run.Gateway, credits *credit.Service, def operations.Definition,
) *operations.Runner {
	return &operations.Runner{
		Svc:  &operations.Service{Pool: pool},
		Step: operations.StepThrough(llm),
		IssueKey: func(ctx context.Context, runID string, budgetUSD float64, ttl time.Duration) (string, error) {
			grant, err := gateway.IssueAgentRun(ctx, runID, run.CreationKeyTerms{TTL: ttl, BudgetUSD: budgetUSD, Model: def.ModelRole})
			if err != nil {
				return "", err
			}
			return grant.VirtualKey, nil
		},
		RevokeKey:  gateway.Revoke,
		RecordCost: AgentCostRecorder(pool, credits),
		Now:        time.Now,
	}
}

func AgentCostRecorder(pool *pgxpool.Pool, credits *credit.Service) func(context.Context, pgtype.UUID, operations.ModelCall) {
	return func(ctx context.Context, runID pgtype.UUID, call operations.ModelCall) {
		e := credit.CostEvent{
			Kind: credit.KindPlatformAgent, Model: call.Model, PromptVersion: call.PromptVersion,
			PromptTokens: call.PromptTokens, CompletionTokens: call.CompletionTokens,
			RefType: credit.RefPlatformAgentRun, RefID: runID,
			IdempotencyKey: string(credit.KindPlatformAgent) + ":" + uuid.NewString(),
		}
		e.UsdMicros, e.Estimated = credit.UsageCost(call.CostUSD)
		if _, _, err := credits.RecordCost(ctx, pool, e); err != nil {
			slog.Warn("platform agent model call cost not recorded", "error", err)
		}
	}
}
