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
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const (
	agentMaxSteps       = 6
	agentMaxTokens      = 60_000
	agentDeadline       = 5 * time.Minute
	agentStepTimeout    = 90 * time.Second
	agentMaxOutputToken = 4_000
)

var AgentLimits = agentloop.Limits{
	MaxSteps: agentMaxSteps, MaxTokens: agentMaxTokens, Deadline: agentDeadline,
	StepTimeout: agentStepTimeout, MaxOutputTokens: agentMaxOutputToken,
}

func AgentTools(
	pool *pgxpool.Pool, rate capacity.RestoreRate, docket func(context.Context) ([]publishing.DocketEntry, error),
) []agentloop.Tool {
	return []agentloop.Tool{MaintenanceReportTool(pool, rate, time.Now), ExposureQueueTool(docket)}
}

func NewAgentRuns(
	pool *pgxpool.Pool, llm *llmclient.Client, gateway *run.Gateway, credits *credit.Service, tools []agentloop.Tool,
) func(ctx context.Context, agent string) error {
	return func(ctx context.Context, agent string) error {
		def, ok := operations.Lookup(agent)
		if !ok {
			return fmt.Errorf("%w: %s", operations.ErrUnknownAgent, agent)
		}
		svc := &operations.Service{Pool: pool}
		loopAgent := svc.LoopAgent(def, MaintenanceActions(pool))
		report, err := NewAgentRunner(svc, llm, gateway, credits, def.ModelRole).Run(ctx, loopAgent, tools, AgentLimits)
		if errors.Is(err, agentloop.ErrHalted) {
			return nil
		}
		if err == nil {
			slog.Info("platform agent run finished", "agent", agent, "status", report.Status, "reason", report.Reason)
		}
		return err
	}
}

func NewAgentRunner(
	journal *operations.Service, llm *llmclient.Client, gateway *run.Gateway, credits *credit.Service, modelRole string,
) *agentloop.Runner {
	return &agentloop.Runner{
		Journal: journal,
		Step:    agentloop.StepThrough(llm),
		IssueKey: func(ctx context.Context, runID string, budgetUSD float64, ttl time.Duration) (string, error) {
			grant, err := gateway.IssueAgentRun(ctx, runID, run.CreationKeyTerms{TTL: ttl, BudgetUSD: budgetUSD, Model: modelRole})
			if err != nil {
				return "", err
			}
			return grant.VirtualKey, nil
		},
		RevokeKey:  gateway.Revoke,
		RecordCost: AgentCostRecorder(journal.Pool, credits),
		Now:        time.Now,
	}
}

func AgentCostRecorder(pool *pgxpool.Pool, credits *credit.Service) func(context.Context, pgtype.UUID, int, agentloop.ModelCall) error {
	return func(ctx context.Context, runID pgtype.UUID, seq int, call agentloop.ModelCall) error {
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
