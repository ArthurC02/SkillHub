package operations

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	stoppedByBrake   = "the agent brake is engaged"
	stoppedByDisable = "the agent was disabled"
)

const usdMicrosPerDollar = 1_000_000

func (s *Service) StartRun(ctx context.Context, agent string) (pgtype.UUID, error) {
	id, err := gen.New(s.Pool).StartPlatformAgentRun(ctx, agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, agentloop.ErrHalted
	}
	return id, err
}

func (s *Service) BeforeStep(ctx context.Context, run pgtype.UUID) error {
	gate, err := gen.New(s.Pool).GetPlatformAgentRunGate(ctx, run)
	if err != nil {
		return err
	}
	if agentloop.Status(gate.Status) != agentloop.Running {
		return agentloop.ErrRunFinished
	}
	reason := haltReason(gate.Enabled, gate.Braked)
	if reason == "" {
		return nil
	}
	if err := s.Finish(ctx, run, agentloop.Ending{Status: agentloop.Stopped, Reason: reason}); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", agentloop.ErrHalted, reason)
}

func haltReason(enabled, braked bool) string {
	switch {
	case braked:
		return stoppedByBrake
	case !enabled:
		return stoppedByDisable
	default:
		return ""
	}
}

func (s *Service) Finish(ctx context.Context, run pgtype.UUID, end agentloop.Ending) error {
	var why *string
	if end.Reason != "" {
		why = &end.Reason
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		finished, err := gen.New(tx).FinishPlatformAgentRun(ctx, gen.FinishPlatformAgentRunParams{
			ID: run, Status: string(end.Status), Reason: why, Result: end.Result,
		})
		if err != nil {
			return err
		}
		if finished == 0 {
			return agentloop.ErrRunFinished
		}
		if end.Record == nil {
			return nil
		}
		return end.Record(ctx, tx, run, end.Now)
	})
}

func (s *Service) SpentSince(ctx context.Context, agent string, since time.Time) (int64, error) {
	runs, err := gen.New(s.Pool).PlatformAgentSpendSince(ctx, gen.PlatformAgentSpendSinceParams{
		Name: agent, Since: pgconv.Timestamptz(since),
	})
	var spent int64
	for _, run := range runs {
		spent += countedSpend(run)
	}
	return spent, err
}

func countedSpend(run gen.PlatformAgentSpendSinceRow) int64 {
	if run.HasUnpriced && run.KeyBudgetMicros != nil {
		return max(run.PricedMicros, *run.KeyBudgetMicros)
	}
	return run.PricedMicros
}

func (s *Service) RecordStep(ctx context.Context, run pgtype.UUID, seq int, step agentloop.StepRecord, model agentloop.ModelCall) error {
	return gen.New(s.Pool).RecordPlatformAgentStep(ctx, gen.RecordPlatformAgentStepParams{
		RunID: run, Seq: int32(seq), Tool: step.Tool, Arguments: step.Arguments, Result: step.Result,
		Model: model.Model, PromptTokens: model.PromptTokens, CompletionTokens: model.CompletionTokens,
		UsdMicros: usdMicros(model.CostUSD),
	})
}

func (s *Service) RecordKeyBudget(ctx context.Context, run pgtype.UUID, budgetUSD float64) error {
	return gen.New(s.Pool).SetPlatformAgentRunKeyBudget(ctx, gen.SetPlatformAgentRunKeyBudgetParams{
		KeyBudgetMicros: usdMicros(&budgetUSD), ID: run,
	})
}

func usdMicros(costUSD *float64) *int64 {
	if costUSD == nil {
		return nil
	}
	micros := int64(math.Round(*costUSD * usdMicrosPerDollar))
	return &micros
}
