package apiserver

import (
	"context"

	"github.com/google/uuid"
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

func (l *creditLedger) Balance(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return l.svc.Balance(ctx, userID)
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

	kind := credit.EntryGrant
	if amountCredits < 0 {
		kind = credit.EntryAdjustment
	}
	balance, err := l.svc.Grant(ctx, tx, credit.GrantInput{
		UserID:         userID,
		EntryKind:      kind,
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
