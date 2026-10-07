package operations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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
	return s.finishWithResult(ctx, run, status, reason, nil)
}

func (s *Service) finishWithResult(ctx context.Context, run pgtype.UUID, status RunStatus, reason string, result []byte) error {
	var why *string
	if reason != "" {
		why = &reason
	}
	finished, err := gen.New(s.Pool).FinishPlatformAgentRun(ctx, gen.FinishPlatformAgentRunParams{
		ID: run, Status: string(status), Reason: why, Result: result,
	})
	if err != nil {
		return err
	}
	if finished == 0 {
		return ErrRunFinished
	}
	return nil
}
