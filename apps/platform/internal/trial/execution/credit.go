package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var ErrCreditBalance = errors.New("run: the workspace has too few credits to start a run")

func (s *Service) requireCredit(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.Ledger == nil {
		return nil
	}
	ok, err := s.Ledger.Reserve(ctx, tx, workspaceID, s.Deployment.Budget())
	if err != nil {
		return err
	}
	if !ok {
		return refused(ReasonCreditBalance, ErrCreditBalance)
	}
	return nil
}

func (s *Service) settleCredit(ctx context.Context, run gen.Run, attempts []gen.RunAttempt) error {
	if s.Ledger == nil || len(attempts) == 0 {

		return nil
	}
	costUSD := s.reportedSpend(ctx, run, attempts)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("start settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	settlement := RunSettlement{WorkspaceID: run.WorkspaceID, RunID: run.ID, CostUSD: costUSD, ReservedUSD: s.Deployment.Budget()}
	if err := s.Ledger.Settle(ctx, tx, settlement); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settlement: %w", err)
	}
	return nil
}

func (s *Service) reportedSpend(ctx context.Context, run gen.Run, attempts []gen.RunAttempt) *float64 {
	if s.Gateway == nil {
		return nil
	}
	var spent float64
	reported := false
	for _, attempt := range attempts {
		usage, err := s.Gateway.Usage(ctx, pgconv.UUIDString(attempt.ID), attemptUsageSince(attempt))
		if err != nil {
			metrics.RunTokenUsageUnreadable.Inc()
			slog.Warn("could not read this attempt's spend; the run is settled once every attempt's spend can be read",
				"run_id", pgconv.UUIDString(run.ID),
				"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
			return nil
		}
		if usage.Incomplete {
			metrics.RunTokenUsageUnreadable.Inc()
			slog.Warn("this attempt's spend log runs past what is read; it is charged only for the calls read",
				"run_id", pgconv.UUIDString(run.ID), "run_attempt_id", pgconv.UUIDString(attempt.ID))
		}
		if usage.CostReported {
			spent += usage.ModelCostUSD
			reported = true
		}
	}
	if !reported {
		return nil
	}
	return &spent
}

const unrecordedAttemptLookback = time.Hour

func attemptUsageSince(attempt gen.RunAttempt) time.Time {
	if attempt.CreatedAt.Valid {
		return attempt.CreatedAt.Time.UTC()
	}
	return time.Now().UTC().Add(-unrecordedAttemptLookback)
}
