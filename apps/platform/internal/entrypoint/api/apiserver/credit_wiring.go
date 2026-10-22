package apiserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
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

func (l *creditLedger) Grant(ctx context.Context, workspaceID pgtype.UUID, grant CreditGrant) (int64, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	idempotencyKey := "grant:" + uuid.NewString()
	if grant.ClientKey != "" {
		idempotencyKey = "grant:" + pgconv.UUIDString(userID) + ":" + grant.ClientKey
	}
	balance, err := l.svc.Grant(ctx, tx, credit.GrantInput{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		EntryKind:      credit.OperatorEntryKind(grant.AmountCredits),
		Credits:        grant.AmountCredits,
		Reason:         grant.Reason,
		OperatorID:     grant.ActorUserID,
		IdempotencyKey: idempotencyKey,
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

func wireSuggestCredit(lab *testlab.Service, svc *credit.Service, owners *identity.Service, pool *pgxpool.Pool) {
	lab.Billing.CanStart = func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		userID, err := owners.WorkspaceOwner(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		check, err := svc.CanStart(ctx, userID, credit.KindSuggestCriteria)
		if err != nil {
			return false, err
		}
		return check.OK, nil
	}
	lab.Billing.Charge = func(ctx context.Context, workspaceID pgtype.UUID, usage *testlab.ModelUsage) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			userID, err := owners.WorkspaceOwnerIn(ctx, tx, workspaceID)
			if err != nil {
				return err
			}
			in := credit.ChargeInput{
				Kind:   credit.KindSuggestCriteria,
				UserID: userID, WorkspaceID: workspaceID,
				IdempotencyKey: string(credit.KindSuggestCriteria) + ":" + uuid.NewString(),
			}
			if usage != nil {
				in.PromptTokens, in.CompletionTokens = usage.PromptTokens, usage.CompletionTokens
				if usd := usage.ReportedCostUSD(); usd != nil {
					micros, _ := credit.BillableMicros(*usd)
					in.UsdMicros = &micros
				}
			}
			_, err = svc.Charge(ctx, tx, in)
			return err
		})
	}
}

func wireGenerateCredit(target *ingest.Service, svc *credit.Service, owners *identity.Service, pool *pgxpool.Pool) {
	target.Billing.CanStart = func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		userID, err := owners.WorkspaceOwner(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		check, err := svc.CanStart(ctx, userID, credit.KindGenerate)
		if err != nil {
			return false, err
		}
		return check.OK, nil
	}
	target.Billing.Charge = func(ctx context.Context, charge ingest.GenerationCharge) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			userID, err := owners.WorkspaceOwnerIn(ctx, tx, charge.WorkspaceID)
			if err != nil {
				return err
			}
			_, err = svc.Charge(ctx, tx, generationChargeInput(charge, userID))
			return err
		})
	}
}

func generationChargeInput(charge ingest.GenerationCharge, userID pgtype.UUID) credit.ChargeInput {
	in := credit.ChargeInput{
		Kind:  credit.KindGenerate,
		Model: charge.Model, PromptVersion: charge.PromptVersion,
		UserID: userID, WorkspaceID: charge.WorkspaceID, IdempotencyKey: charge.IdempotencyKey,
	}
	if u := charge.Usage; u != nil {
		in.PromptTokens, in.CompletionTokens = u.PromptTokens, u.CompletionTokens
		if usd := u.ReportedCostUSD(); usd != nil {
			micros, _ := credit.BillableMicros(*usd)
			in.UsdMicros = &micros
		}
	}
	return in
}

func (l *creditLedger) Ledger(ctx context.Context, workspaceID, operatorID pgtype.UUID) (credit.Ledger, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return credit.Ledger{}, err
	}
	var ledger credit.Ledger
	err = pgx.BeginFunc(ctx, l.pool, func(tx pgx.Tx) error {
		var err error
		ledger, err = l.svc.Ledger(ctx, tx, credit.LedgerQuery{
			Account: userID, Workspace: workspaceID, Operator: operatorID,
		})
		return err
	})
	return ledger, err
}

func (l *creditLedger) Statement(
	ctx context.Context, workspaceID pgtype.UUID, beforeAt time.Time, beforeID pgtype.UUID,
) ([]credit.StatementEntry, bool, error) {
	userID, err := l.owner(ctx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	return l.svc.Statement(ctx, userID, beforeAt, beforeID)
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
