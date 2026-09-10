package run

// A Run is gated on the account covering its gateway ceiling and charged its
// actual spend at cleanup. Both calls are injected funcs: run may not import credit.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// ErrCreditBalance refuses a Run the account cannot cover.
var ErrCreditBalance = errors.New("點數不足，無法開始這次試跑。請聯絡管理者為這個帳號加點；已經開始的試跑不受影響。")

// usdMicros converts dollars to ledger micros, rounding up.
func usdMicros(usd float64) int64 {
	micros := usd * 1_000_000
	whole := int64(micros)
	if float64(whole) < micros {
		whole++
	}
	return whole
}

// requireCredit runs on create()'s tx after requireRunSlot's lock, so two
// simultaneous requests cannot both reserve the last of a balance.
func (s *Service) requireCredit(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	if s.CreditReserve == nil {
		return nil
	}
	ok, err := s.CreditReserve(ctx, tx, workspaceID, usdMicros(RunBudgetUSD()))
	if err != nil {
		return err
	}
	if !ok {
		return refused("credit_balance", ErrCreditBalance)
	}
	return nil
}

// settleCredit runs before cleanup revokes the keys, since a revoked key's
// spend is unreadable. It never fails cleanup; the charge is idempotent.
func (s *Service) settleCredit(ctx context.Context, run gen.Run, attempts []gen.RunAttempt) {
	if s.CreditSettle == nil || len(attempts) == 0 {
		// No attempts: no key was minted, nothing to settle.
		return
	}
	var spent float64
	reported := false
	if s.Gateway != nil {
		for _, attempt := range attempts {
			since := time.Now().UTC().Add(-time.Hour)
			if attempt.CreatedAt.Valid {
				since = attempt.CreatedAt.Time.UTC()
			}
			usage, err := s.Gateway.AttemptUsage(ctx, pgconv.UUIDString(attempt.ID), since)
			if err != nil {
				// An unread spend would look like a free run, so count it.
				metrics.RunTokenUsageUnreadable.Inc()
				slog.Warn("could not read this attempt's spend; it will not be charged",
					"run_id", pgconv.UUIDString(run.ID),
					"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
				continue
			}
			if usage.SpendReported {
				spent += usage.SpendUSD
				reported = true
			}
		}
	}
	var micros *int64
	if reported {
		v := usdMicros(spent)
		micros = &v
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		slog.Error("run credit settlement could not start", "run_id", pgconv.UUIDString(run.ID), "error", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.CreditSettle(ctx, tx, run.WorkspaceID, run.ID, micros, usdMicros(RunBudgetUSD())); err != nil {
		// Never block teardown on the ledger; the supervisor's next sweep retries.
		slog.Error("run credit settlement failed", "run_id", pgconv.UUIDString(run.ID), "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("run credit settlement did not commit", "run_id", pgconv.UUIDString(run.ID), "error", err)
	}
}
