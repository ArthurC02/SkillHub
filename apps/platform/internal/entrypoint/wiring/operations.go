package wiring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const (
	agentMaxSteps       = 6
	agentMaxTokens      = 60_000
	agentDeadline       = 5 * time.Minute
	agentStepTimeout    = 90 * time.Second
	agentMaxOutputToken = 4_000
)

var AgentLimits = operations.Limits{
	MaxSteps: agentMaxSteps, MaxTokens: agentMaxTokens, Deadline: agentDeadline,
	StepTimeout: agentStepTimeout, MaxOutputTokens: agentMaxOutputToken,
}

func AgentTools(pool *pgxpool.Pool, rate capacity.RestoreRate) []operations.Tool {
	return []operations.Tool{MaintenanceReportTool(pool, rate, time.Now)}
}

func NewAgentRuns(
	pool *pgxpool.Pool, llm *llmclient.Client, gateway *run.Gateway, credits *credit.Service, rate capacity.RestoreRate,
) func(ctx context.Context, agent string) error {
	tools := AgentTools(pool, rate)
	return func(ctx context.Context, agent string) error {
		def, ok := operations.Lookup(agent)
		if !ok {
			return fmt.Errorf("%w: %s", operations.ErrUnknownAgent, agent)
		}
		report, err := NewAgentRunner(pool, llm, gateway, credits, def).Run(ctx, def, tools, AgentLimits)
		if errors.Is(err, operations.ErrAgentHalted) {
			return nil
		}
		if err == nil {
			slog.Info("platform agent run finished", "agent", agent, "status", report.Status, "reason", report.Reason)
		}
		return err
	}
}

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
		Actions:    MaintenanceActions(pool),
	}
}

func AgentCostRecorder(pool *pgxpool.Pool, credits *credit.Service) func(context.Context, pgtype.UUID, int, operations.ModelCall) error {
	return func(ctx context.Context, runID pgtype.UUID, seq int, call operations.ModelCall) error {
		e := credit.CostEvent{
			Kind: credit.KindPlatformAgent, Model: call.Model, PromptVersion: call.PromptVersion,
			PromptTokens: call.PromptTokens, CompletionTokens: call.CompletionTokens,
			RefType: credit.RefPlatformAgentRun, RefID: runID,
			IdempotencyKey: fmt.Sprintf("%s:%s:%d", credit.KindPlatformAgent, pgconv.UUIDString(runID), seq),
		}
		e.UsdMicros, e.Estimated = credit.UsageCost(call.CostUSD)
		_, _, err := credits.RecordCost(ctx, pool, e)
		return err
	}
}
