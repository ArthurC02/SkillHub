package operations

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	return s.finishWithResult(ctx, run, runEnding{status: status, reason: reason})
}

type trackedFindings struct {
	sightings []Sighting
	now       time.Time
}

type runEnding struct {
	status   RunStatus
	reason   string
	result   []byte
	findings *trackedFindings
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
		if end.findings == nil {
			return nil
		}
		return s.recordFindings(ctx, tx, run, end.findings.sightings, end.findings.now)
	})
}
