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
	if s.CreditReserve == nil {
		return nil
	}
	ok, err := s.CreditReserve(ctx, tx, workspaceID, s.Deployment.Budget())
	if err != nil {
		return err
	}
	if !ok {
		return refused(ReasonCreditBalance, ErrCreditBalance)
	}
	return nil
}

func (s *Service) settleCredit(ctx context.Context, run gen.Run, attempts []gen.RunAttempt) error {
	if s.CreditSettle == nil || len(attempts) == 0 {

		return nil
	}
	var spent float64
	reported := false
	if s.Gateway != nil {
		for _, attempt := range attempts {
			since := time.Now().UTC().Add(-time.Hour)
			if attempt.CreatedAt.Valid {
				since = attempt.CreatedAt.Time.UTC()
			}
			usage, err := s.Gateway.Usage(ctx, pgconv.UUIDString(attempt.ID), since)
			if err != nil {

				metrics.RunTokenUsageUnreadable.Inc()
				slog.Warn("could not read this attempt's spend; it will not be charged",
					"run_id", pgconv.UUIDString(run.ID),
					"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
				continue
			}
			if usage.CostReported {
				spent += usage.ModelCostUSD
				reported = true
			}
		}
	}
	var costUSD *float64
	if reported {
		costUSD = &spent
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("start settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.CreditSettle(ctx, tx, run.WorkspaceID, run.ID, costUSD, s.Deployment.Budget()); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settlement: %w", err)
	}
	return nil
}
