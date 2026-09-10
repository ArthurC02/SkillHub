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
)

// This file is the one place the two id spaces meet.
//
// credit keys accounts on the USER (migration 0060: "每個帳號" is the user).
// Everything that spends carries a WORKSPACE id: creation's job args, the
// operator grant route's path parameter, this table's handlers. ADR-011 gives
// each account exactly one personal workspace today, which makes the mapping
// total but does NOT make the two ids the same value — they are different
// columns, and passing one where the other belongs would charge the wrong
// account the day that stops being 1:1.
//
// So the resolution happens here, in the composition root, exactly once per
// seam, through identity.WorkspaceOwner. Neither credit nor creation imports
// the other, and neither learns about the other's id space (ADR-032 §1's
// Facts convention, ADR-068's "credit 本身不 import identity").

// creditLedger adapts credit.Service to this table's CreditLedger port,
// translating workspace ids to the user ids the ledger is keyed on.
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

// SessionEstimate reports the interactive-creation window. The threshold it
// carries is the same one credit.Service.CanStart blocks against — one
// number, one definition, so the screen and the gate cannot disagree.
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

// Grant runs the operator's top-up and its audit event in one transaction —
// credit.Service.Grant writes both, so a granted balance with no audit trail
// is not a state this can end in (02:SEC-011「誰做的」).
//
// The idempotency key is generated per request. That is a deliberate, named
// limitation rather than an oversight: credit_entries is idempotent on this
// key, but nothing in the HTTP request identifies a retry, so two identical
// operator POSTs are two grants. It is the honest shape for MVP — an
// operator grant is a deliberate act and each one lands in the audit log —
// and the fix, when a client needs it, is an idempotency key on the request
// rather than anything in this file.
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

// newCreditService builds the ledger this deployment charges against.
//
// Facts is wired, not left nil: credit.Service treats a nil Facts as "no
// identity to ask" and skips the account check entirely, which is only
// acceptable for a composition root that has no identity service. This one
// has one.
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

// wireCreationCredit assigns creation's three gates. Each one resolves the
// workspace it is handed to the account that pays for it, then asks credit.
//
// Left unwired, all three are nil and creation runs exactly as it did before
// ADR-068 — which is why they must be assigned here rather than defaulted
// anywhere: a gate that silently does not run is the failure mode 04 乙-2
// names, one layer down.
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
		// (session, revision) is the idempotency key ADR-068 decision 5 names:
		// a Worker retry settling the same revision debits once. It is derived
		// from the settlement's own identity rather than generated, which is
		// exactly what the operator grant above cannot do.
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

// wireCostRecording hands the ledger to the contexts that make paid calls of
// their own (CRED-005). None of them asks it a question — catalog and ingest
// only write what a call cost — so this is assignment, not a gate, and a
// deployment that skipped it degrades to a ledger with holes rather than to a
// platform that refuses to search or import.
//
// The three services are the ones this process actually holds. eval's paid
// calls all happen in the Worker, which wires its own (worker/credit_wiring.go).
func wireCostRecording(svc *credit.Service, search *catalog.Service, versions *ingest.Service) {
	search.Credit = svc
	versions.Credit = svc
}
