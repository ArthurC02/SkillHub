package wiring

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func NewCreditService(pool *pgxpool.Pool) (*credit.Service, error) {
	cfg, err := CreditConfigFromEnv()
	if err != nil {
		return nil, err
	}
	ids := &identity.Service{Pool: pool}
	return &credit.Service{
		Store:  credit.NewPostgresStore(pool),
		Config: cfg,
		Facts: func(ctx context.Context, db credit.DBTX, userID pgtype.UUID) (credit.AccountFacts, error) {
			if db == nil {
				db = pool
			}
			present, purging, err := ids.AccountStateIn(ctx, db, userID)
			if err != nil {
				return credit.AccountFacts{}, err
			}
			return credit.AccountFacts{Exists: present, Purged: purging}, nil
		},
	}, nil
}

func CreditConfigFromEnv() (credit.Config, error) {
	usd, err := creditEnvFloat("CREDIT_USD_PER_CREDIT", 0.001)
	if err != nil {
		return credit.Config{}, fmt.Errorf("credit: CREDIT_USD_PER_CREDIT: %w", err)
	}
	markup, err := creditEnvInt("CREDIT_MARKUP_BPS", 13000)
	if err != nil {
		return credit.Config{}, fmt.Errorf("credit: CREDIT_MARKUP_BPS: %w", err)
	}
	floor, err := creditEnvInt("CREDIT_DEBT_FLOOR", -50)
	if err != nil {
		return credit.Config{}, fmt.Errorf("credit: CREDIT_DEBT_FLOOR: %w", err)
	}
	fallback, err := creditEnvInt("CREDIT_MIN_START_FALLBACK", 70)
	if err != nil {
		return credit.Config{}, fmt.Errorf("credit: CREDIT_MIN_START_FALLBACK: %w", err)
	}
	return credit.NewConfig(usd, markup, floor, fallback)
}

func creditEnvFloat(name string, fallback float64) (float64, error) {
	if raw := os.Getenv(name); raw != "" {
		return strconv.ParseFloat(raw, 64)
	}
	return fallback, nil
}

func creditEnvInt(name string, fallback int64) (int64, error) {
	if raw := os.Getenv(name); raw != "" {
		return strconv.ParseInt(raw, 10, 64)
	}
	return fallback, nil
}

func billable(usd float64) int64 {
	micros, _ := credit.BillableMicros(usd)
	return micros
}

func billableOrNil(usd *float64) *int64 {
	if usd == nil {
		return nil
	}
	micros := billable(*usd)
	return &micros
}

func WireCreationCredit(target *creation.Service, svc *credit.Service, pool *pgxpool.Pool) {
	ids := &identity.Service{Pool: pool}
	owner := ids.WorkspaceOwner

	target.Billing = creation.BillingHooks{
		CanStartFunc: func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
			userID, err := owner(ctx, workspaceID)
			if err != nil {
				return false, err
			}
			check, err := svc.CanStart(ctx, userID, credit.KindCreationSession)
			if err != nil {
				return false, err
			}
			return check.OK, nil
		},
		SessionEndedFunc: svc.SummarizeSession,
		ReserveFunc: func(ctx context.Context, workspaceID pgtype.UUID, reservedUSD float64) (bool, error) {
			userID, err := owner(ctx, workspaceID)
			if err != nil {
				return false, err
			}
			return svc.CanAffordStep(ctx, userID, billable(reservedUSD))
		},
		SettleFunc: func(ctx context.Context, tx pgx.Tx, workspaceID, sessionID pgtype.UUID, revision int64, costUSD *float64, reservedUSD float64) error {
			userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
			if err != nil {
				return err
			}

			_, err = svc.Charge(ctx, tx, credit.ChargeInput{
				Kind:              credit.KindCreationStep,
				UsdMicros:         billableOrNil(costUSD),
				ReservedUsdMicros: billable(reservedUSD),
				UserID:            userID,
				WorkspaceID:       workspaceID,
				RefType:           credit.RefCreationSession,
				RefID:             sessionID,
				IdempotencyKey:    fmt.Sprintf("creation:%s:%d", pgconv.UUIDString(sessionID), revision),
			})
			return err
		},
	}
}

func WireCreditDisplay(svc *credit.Service, runs *run.Service, traces *trace.Service, evaluations *eval.Service) {
	traces.Credits = svc.CreditsForUSD
	evaluations.Credits = svc.CreditsForUSD
}

func WireRunCredit(target *run.Service, svc *credit.Service, pool *pgxpool.Pool) {
	target.Ledger = runCreditLedger{credits: svc, identities: &identity.Service{Pool: pool}}
}

type runCreditLedger struct {
	credits    *credit.Service
	identities *identity.Service
}

func (l runCreditLedger) CreditsForUSD(usd float64) (int64, bool) {
	return l.credits.CreditsForUSD(usd)
}

func (l runCreditLedger) Reserve(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSD float64) (bool, error) {
	userID, err := l.identities.WorkspaceOwnerIn(ctx, tx, workspaceID)
	if err != nil {
		return false, err
	}
	return l.credits.CanAffordStepIn(ctx, tx, userID, billable(reservedUSD))
}

func (l runCreditLedger) Settle(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, costUSD *float64, reservedUSD float64) error {
	userID, err := l.identities.WorkspaceOwnerIn(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	key := "run:" + pgconv.UUIDString(runID)
	if costUSD == nil {

		_, _, err := l.credits.RecordCost(ctx, tx, credit.CostEvent{
			Kind:           credit.KindRun,
			Estimated:      true,
			WorkspaceID:    workspaceID,
			UserID:         userID,
			RefType:        credit.RefRun,
			RefID:          runID,
			IdempotencyKey: key + ":unreadable",
		})
		return err
	}
	_, err = l.credits.Charge(ctx, tx, credit.ChargeInput{
		Kind:              credit.KindRun,
		UsdMicros:         billableOrNil(costUSD),
		ReservedUsdMicros: billable(reservedUSD),
		UserID:            userID,
		WorkspaceID:       workspaceID,
		RefType:           credit.RefRun,
		RefID:             runID,

		IdempotencyKey: key,
	})
	return err
}
