package apiserver

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
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
	est, err := l.svc.Estimate(ctx, credit.KindCreationStep)
	if err != nil {
		return CreditSessionEstimate{}, err
	}
	return CreditSessionEstimate{
		LowCredits:       est.LowCredits,
		HighCredits:      est.HighCredits,
		ThresholdCredits: est.ThresholdCredits,
		SampleSize:       est.SampleCount,
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
		EntryKind:      credit.EntryGrant,
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

func newCreditService(pool *pgxpool.Pool, identitySvc *identity.Service) (*credit.Service, error) {
	cfg, err := credit.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return &credit.Service{
		Store:  credit.NewPostgresStore(pool),
		Config: cfg,
		Facts: func(ctx context.Context, userID pgtype.UUID) (credit.AccountFacts, error) {
			present, purging, err := identitySvc.AccountState(ctx, userID)
			if err != nil {
				return credit.AccountFacts{}, err
			}
			return credit.AccountFacts{Exists: present, Purged: purging}, nil
		},
	}, nil
}

func wireCreationCredit(
	target *creation.Service,
	svc *credit.Service,
	owner func(ctx context.Context, workspaceID pgtype.UUID) (pgtype.UUID, error),
) {
	target.CreditCanStart = func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		userID, err := owner(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		check, err := svc.CanStart(ctx, userID, credit.KindCreationStep)
		if err != nil {
			return false, err
		}
		return check.OK, nil
	}
	target.CreditReserve = func(ctx context.Context, workspaceID pgtype.UUID, reservedUSDMicros int64) (bool, error) {
		userID, err := owner(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		return svc.CanAffordStep(ctx, userID, reservedUSDMicros)
	}
	target.CreditSettle = func(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, usdMicros *int64, reservedUSDMicros int64) error {
		userID, err := owner(ctx, workspaceID)
		if err != nil {
			return err
		}

		key := fmt.Sprintf("creation:%s:%d", pgconv.UUIDString(sessionID), revision)
		_, err = svc.Charge(ctx, tx, credit.ChargeInput{
			Kind:              credit.KindCreationStep,
			UsdMicros:         usdMicros,
			ReservedUsdMicros: reservedUSDMicros,
			UserID:            userID,
			WorkspaceID:       workspaceID,
			RefType:           credit.RefCreationSession,
			RefID:             sessionID,
			IdempotencyKey:    key,
		})
		return err
	}
}

func wireCostRecording(svc *credit.Service, search *catalog.Service, versions *ingest.Service) {
	search.Credit = svc
	versions.Credit = svc
}

func wireCreditDisplay(svc *credit.Service, runs *run.Service, traces *trace.Service, evaluations *eval.Service) {
	runs.Credits = svc.CreditsForUSD
	traces.Credits = svc.CreditsForUSD
	evaluations.Credits = svc.CreditsForUSD
}

func wireRunCredit(target *run.Service, svc *credit.Service, pool *pgxpool.Pool) {
	ids := &identity.Service{Pool: pool}

	target.CreditReserve = func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSDMicros int64) (bool, error) {

		userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return false, err
		}
		return svc.CanAffordStepIn(ctx, tx, userID, reservedUSDMicros)
	}
	target.CreditSettle = func(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, usdMicros *int64, reservedUSDMicros int64) error {
		userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		key := "run:" + pgconv.UUIDString(runID)
		if usdMicros == nil {

			_, _, err := svc.RecordCost(ctx, tx, credit.CostEvent{
				Kind:           credit.KindRun,
				Estimated:      true,
				WorkspaceID:    workspaceID,
				UserID:         userID,
				RefType:        credit.RefRun,
				RefID:          runID,
				IdempotencyKey: key,
			})
			return err
		}
		_, err = svc.Charge(ctx, tx, credit.ChargeInput{
			Kind:              credit.KindRun,
			UsdMicros:         usdMicros,
			ReservedUsdMicros: reservedUSDMicros,
			UserID:            userID,
			WorkspaceID:       workspaceID,
			RefType:           credit.RefRun,
			RefID:             runID,

			IdempotencyKey: key,
		})
		return err
	}
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
