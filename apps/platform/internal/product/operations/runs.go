package operations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type RunStatus string

const (
	RunRunning    RunStatus = "running"
	RunCompleted  RunStatus = "completed"
	RunIncomplete RunStatus = "incomplete"
	RunStopped    RunStatus = "stopped"
	RunFailed     RunStatus = "failed"
)

const (
	stoppedByBrake   = "the agent brake is engaged"
	stoppedByDisable = "the agent was disabled"
)

var ErrAgentHalted = errors.New("operations: the agent is disabled or the agent brake is engaged")

var ErrRunFinished = errors.New("operations: the run has already finished")

func (s *Service) StartRun(ctx context.Context, agent string) (pgtype.UUID, error) {
	id, err := gen.New(s.Pool).StartPlatformAgentRun(ctx, agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, ErrAgentHalted
	}
	return id, err
}

func (s *Service) BeforeStep(ctx context.Context, run pgtype.UUID) error {
	gate, err := gen.New(s.Pool).GetPlatformAgentRunGate(ctx, run)
	if err != nil {
		return err
	}
	if RunStatus(gate.Status) != RunRunning {
		return ErrRunFinished
	}
	reason := haltReason(gate.Enabled, gate.Braked)
	if reason == "" {
		return nil
	}
	if err := s.FinishRun(ctx, run, RunStopped, reason); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrAgentHalted, reason)
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

func (s *Service) FinishRun(ctx context.Context, run pgtype.UUID, status RunStatus, reason string) error {
	return s.finishWithResult(ctx, run, runEnding{status: status, reason: reason})
}

type runEnding struct {
	status         RunStatus
	reason         string
	result         []byte
	tracksFindings bool
	sightings      []Sighting
	proposals      []preparedProposal
	now            time.Time
}

func (s *Service) finishWithResult(ctx context.Context, run pgtype.UUID, end runEnding) error {
	var why *string
	if end.reason != "" {
		why = &end.reason
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		finished, err := gen.New(tx).FinishPlatformAgentRun(ctx, gen.FinishPlatformAgentRunParams{
			ID: run, Status: string(end.status), Reason: why, Result: end.result,
		})
		if err != nil {
			return err
		}
		if finished == 0 {
			return ErrRunFinished
		}
		if end.tracksFindings {
			if err := s.recordFindings(ctx, tx, run, end.sightings, end.now); err != nil {
				return err
			}
		}
		return s.recordProposals(ctx, tx, run, end.proposals, end.now)
	})
}

func (s *Service) spentSince(ctx context.Context, agent string, since time.Time) (int64, error) {
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

func (s *Service) recordStep(ctx context.Context, run pgtype.UUID, seq int, step StepRecord, model ModelCall) error {
	return gen.New(s.Pool).RecordPlatformAgentStep(ctx, gen.RecordPlatformAgentStepParams{
		RunID: run, Seq: int32(seq), Tool: step.Tool, Arguments: step.Arguments, Result: step.Result,
		Model: model.Model, PromptTokens: model.PromptTokens, CompletionTokens: model.CompletionTokens,
		UsdMicros: usdMicros(model.CostUSD),
	})
}

func (s *Service) recordKeyBudget(ctx context.Context, run pgtype.UUID, budgetUSD float64) error {
	return gen.New(s.Pool).SetPlatformAgentRunKeyBudget(ctx, gen.SetPlatformAgentRunKeyBudgetParams{
		KeyBudgetMicros: usdMicros(&budgetUSD), ID: run,
	})
}
