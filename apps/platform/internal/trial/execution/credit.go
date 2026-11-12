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

func (s *Service) requireCredit(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, budgetUSD float64) error {
	if s.Ledger == nil {
		return nil
	}
	ok, err := s.Ledger.Reserve(ctx, tx, workspaceID, budgetUSD)
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
		return markCostSettled(ctx, s.queries(), run)
	}
	if s.Deployment.spendMayStillLand(run.FinishedAt, s.now()) {
		return nil
	}
	costUSD := s.reportedSpend(ctx, run, attempts)
	reservedUSD, err := reservedBudgetUSD(ctx, s.queries(), run, s.Deployment)
	if err != nil {
		return fmt.Errorf("read the budget the run was admitted under: %w", err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("start settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	settlement := RunSettlement{WorkspaceID: run.WorkspaceID, RunID: run.ID, CostUSD: costUSD, ReservedUSD: reservedUSD}
	if err := s.Ledger.Settle(ctx, tx, settlement); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	if costUSD != nil {
		if err := markCostSettled(ctx, gen.New(tx), run); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settlement: %w", err)
	}
	return nil
}

func reservedBudgetUSD(ctx context.Context, q *gen.Queries, run gen.Run, d Deployment) (float64, error) {
	snapshots, err := q.GetRunSnapshots(ctx, gen.GetRunSnapshotsParams{RunID: run.ID, WorkspaceID: run.WorkspaceID})
	if err != nil {
		return 0, err
	}
	_, policy, err := requirementsFor(snapshots.PolicySnapshot)
	if err != nil {
		return 0, err
	}
	return policy.modelBudget(d), nil
}

const GatewaySpendLandsWithin = 30 * time.Second

func (d Deployment) spendMayStillLand(finished pgtype.Timestamptz, now time.Time) bool {
	return !d.SettlesWithoutSpendWait && finished.Valid && now.Sub(finished.Time) < GatewaySpendLandsWithin
}

func markCostSettled(ctx context.Context, q *gen.Queries, run gen.Run) error {
	return q.MarkRunCostSettled(ctx, gen.MarkRunCostSettledParams{RunID: run.ID, WorkspaceID: run.WorkspaceID})
}

func (s *Service) reportedSpend(ctx context.Context, run gen.Run, attempts []gen.RunAttempt) *float64 {
	if s.Gateway == nil {
		return nil
	}
	var total AttemptUsage
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
			slog.Warn("this attempt's spend log was not fully read; the run is settled once the full spend can be read",
				"run_id", pgconv.UUIDString(run.ID), "run_attempt_id", pgconv.UUIDString(attempt.ID))
			return nil
		}
		if usage.CostIncomplete {
			return nil
		}
		if usage.CostReported {
			total.addCost(usage.ModelCostUSD)
			total.CostReported = true
		}
	}
	if !total.CostReported || total.CostIncomplete {
		return nil
	}
	return &total.ModelCostUSD
}

const unrecordedAttemptLookback = time.Hour

func attemptUsageSince(attempt gen.RunAttempt) time.Time {
	if attempt.CreatedAt.Valid {
		return attempt.CreatedAt.Time.UTC()
	}
	return time.Now().UTC().Add(-unrecordedAttemptLookback)
}
