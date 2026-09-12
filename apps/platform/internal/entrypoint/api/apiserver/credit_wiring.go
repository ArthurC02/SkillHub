package apiserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

type creditLedger struct {
	svc   *credit.Service
	owner func(ctx context.Context, workspaceID pgtype.UUID) (pgtype.UUID, error)
	pool  *pgxpool.Pool
}

var _ CreditLedger = (*creditLedger)(nil)

func (l *creditLedger) Standing(ctx context.Context, workspaceID pgtype.UUID) (int64, bool, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return 0, false, err
	}
	check, err := l.svc.CanStart(ctx, userID, credit.KindCreationSession)
	if err != nil {
		return 0, false, err
	}
	return check.Balance, check.OK, nil
}

func (l *creditLedger) SessionEstimate(ctx context.Context) (CreditSessionEstimate, error) {
	est, err := l.svc.Estimate(ctx, credit.KindCreationSession)
	if err != nil {
		return CreditSessionEstimate{}, err
	}
	return CreditSessionEstimate{
		LowCredits:       est.LowCredits,
		HighCredits:      est.HighCredits,
		ThresholdCredits: est.ThresholdCredits,
		SampleSize:       est.SampleCount,
		DebtFloorCredits: l.svc.Config.DebtFloorCredits,
		Estimated:        est.Estimated,
	}, nil
}

func (l *creditLedger) Grant(ctx context.Context, workspaceID pgtype.UUID, amountCredits int64, reason string, actorUserID pgtype.UUID) (int64, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	balance, err := l.svc.Grant(ctx, tx, credit.GrantInput{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		EntryKind:      credit.OperatorEntryKind(amountCredits),
		Credits:        amountCredits,
		Reason:         reason,
		OperatorID:     actorUserID,
		IdempotencyKey: "grant:" + uuid.NewString(),
	})
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return balance, nil
}

func wireCostRecording(svc *credit.Service, search *catalog.Service, versions *ingest.Service) {
	search.Credit = svc
	versions.Credit = svc
}

func wireGenerateCredit(
	target *ingest.Service,
	svc *credit.Service,
	owner func(ctx context.Context, workspaceID pgtype.UUID) (pgtype.UUID, error),
) {
	target.CreditCanStart = func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		userID, err := owner(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		check, err := svc.CanStart(ctx, userID, credit.KindGenerate)
		if err != nil {
			return false, err
		}
		return check.OK, nil
	}
}

func (l *creditLedger) Ledger(ctx context.Context, workspaceID, operatorID pgtype.UUID) (credit.Ledger, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return credit.Ledger{}, err
	}
	var ledger credit.Ledger
	err = pgx.BeginFunc(ctx, l.pool, func(tx pgx.Tx) error {
		var err error
		ledger, err = l.svc.Ledger(ctx, tx, userID, workspaceID, operatorID)
		return err
	})
	return ledger, err
}

func (l *creditLedger) CostStatistics(ctx context.Context) ([]credit.KindStatistics, error) {
	return l.svc.LatestStatistics(ctx)
}

func (l *creditLedger) DailyCost(ctx context.Context, since time.Time) ([]credit.DailyAmount, error) {
	return l.svc.DailyCost(ctx, since)
}

func (l *creditLedger) DailyCredits(ctx context.Context, since time.Time) ([]credit.DailyAmount, int64, error) {
	return l.svc.DailyCredits(ctx, since)
}
