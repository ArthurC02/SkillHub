package wiring

import (
	"context"
	"fmt"

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
	cfg, err := credit.ConfigFromEnv()
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
	runs.Credits = svc.CreditsForUSD
	traces.Credits = svc.CreditsForUSD
	evaluations.Credits = svc.CreditsForUSD
}

func WireRunCredit(target *run.Service, svc *credit.Service, pool *pgxpool.Pool) {
	ids := &identity.Service{Pool: pool}

	target.CreditReserve = func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSD float64) (bool, error) {

		userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return false, err
		}
		return svc.CanAffordStepIn(ctx, tx, userID, billable(reservedUSD))
	}
	target.CreditSettle = func(ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID, costUSD *float64, reservedUSD float64) error {
		userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		key := "run:" + pgconv.UUIDString(runID)
		if costUSD == nil {

			_, _, err := svc.RecordCost(ctx, tx, credit.CostEvent{
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
		_, err = svc.Charge(ctx, tx, credit.ChargeInput{
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
}
