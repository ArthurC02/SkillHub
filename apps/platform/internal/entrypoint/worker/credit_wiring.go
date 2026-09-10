package worker

import (
	"context"
	"fmt"
	"time"

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

// The Worker's half of ADR-068's gates, and it is the half that spends.
//
// The API mounts the balance and grants credit; it also runs gate ① once,
// when a session is created. But every paid model call happens here — so
// gate ② (is there room for this step) and the settlement that follows it
// (charge what the step actually cost) are only ever reached through this
// composition root. Wiring them in NewApp alone would leave both silently
// nil in the process that does the spending, which is the shape of bug the
// hooks' own doc comments warn about: a gate that does not run is
// indistinguishable from a gate that allowed.
//
// Duplicated rather than shared with apiserver's copy on purpose: each
// composition root assembles its own dependencies, the same way
// creation_wiring.go here does not reuse apiserver's creation wiring.

// newCreditService builds the Worker's ledger. Facts comes from a read-only
// identity service — the purge fields are not set because this process never
// deletes an account, only refuses to charge one that is gone.
func newCreditService(pool *pgxpool.Pool) (*credit.Service, error) {
	cfg, err := credit.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	ids := &identity.Service{Pool: pool}
	return &credit.Service{
		Store:  credit.NewPostgresStore(pool),
		Config: cfg,
		Facts: func(ctx context.Context, userID pgtype.UUID) (credit.AccountFacts, error) {
			present, purging, err := ids.AccountState(ctx, userID)
			if err != nil {
				return credit.AccountFacts{}, err
			}
			return credit.AccountFacts{Exists: present, Purged: purging}, nil
		},
	}, nil
}

// wireCreationCredit assigns creation's three gates, resolving each workspace
// to the account that pays for it first. credit keys accounts on the user;
// creation's job args carry a workspace id; ADR-011's one-workspace-per-user
// makes the mapping total but not the two ids interchangeable, so the lookup
// is explicit here rather than assumed anywhere.
func wireCreationCredit(target *creation.Service, svc *credit.Service, pool *pgxpool.Pool) {
	ids := &identity.Service{Pool: pool}
	owner := ids.WorkspaceOwner

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
		// (session, revision) is decision 5's idempotency key: a River retry
		// settling the same revision debits once. usdMicros nil is settleCost's
		// UsageUnknown branch — Charge then bills the reservation and marks the
		// entry estimated, and never charges zero for a call that happened.
		_, err = svc.Charge(ctx, tx, credit.ChargeInput{
			Kind:              credit.KindCreationStep,
			UsdMicros:         usdMicros,
			ReservedUsdMicros: reservedUSDMicros,
			UserID:            userID,
			WorkspaceID:       workspaceID,
			RefType:           credit.RefCreationSession,
			RefID:             sessionID,
			IdempotencyKey:    fmt.Sprintf("creation:%s:%d", pgconv.UUIDString(sessionID), revision),
		})
		return err
	}
}

// creditStatKinds are the cost-event kinds whose rolling windows the daily
// job recomputes. Every kind the CHECK allows is
// listed: RecomputeArgs carries the kind, so a kind left out of this slice is
// a kind whose statistics are never recomputed — silently, because there is
// no error anywhere for "nobody scheduled this one". Adding a kind to the
// domain means adding it here.
var creditStatKinds = []string{
	credit.KindCreationStep,
	credit.KindSearchEmbedding,
	credit.KindIndexEnrich,
	credit.KindReview,
	credit.KindSuggestion,
	credit.KindGenerate,
	credit.KindRun,
}

// creditStatWindow is how far back each daily recompute looks. Seven days,
// not one: gate ①'s threshold needs at least MinStatSamples points before it
// stops falling back to the configured constant, and a single day of a small
// beta does not reliably reach that. A wider window is the difference between
// a measured p95 and a constant that never gets replaced.
const creditStatWindow = 7 * 24 * time.Hour

// wireCostRecording hands the ledger to the contexts in this process that make
// paid calls of their own (CRED-005).
//
// The Worker's list is longer than the API's by one and it is the important
// one: judging and advising are only ever reached from here, so eval's spend
// is recorded in this process or in none. backfill is the enrichment worker's
// own ingest service — a nil there is a re-enrichment run that costs money and
// leaves no trace of having done so, which is exactly the hole this batch
// exists to close, so it is wired even though it is optional (nil when the
// deployment has no LLM at all).
func wireCostRecording(svc *credit.Service, search *catalog.Service, versions, backfill *ingest.Service, evaluations *eval.Service) {
	search.Credit = svc
	versions.Credit = svc
	if backfill != nil {
		backfill.Credit = svc
	}
	evaluations.Credit = svc
}

// wireCreditDisplay is the API's counterpart (see its comment there). The
// Worker builds the same three contexts, and eval's comparison view is
// assembled here as well as there — a converter wired in one process and not
// the other would make the same field null in half the deployments.
func wireCreditDisplay(svc *credit.Service, runs *run.Service, traces *trace.Service, evaluations *eval.Service) {
	runs.Credits = svc.CreditsForUSD
	traces.Credits = svc.CreditsForUSD
	evaluations.Credits = svc.CreditsForUSD
}

// wireRunCredit assigns both Run hooks in every root, so neither process can
// end up with a nil gate it needed.
func wireRunCredit(target *run.Service, svc *credit.Service, pool *pgxpool.Pool) {
	ids := &identity.Service{Pool: pool}

	target.CreditReserve = func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, reservedUSDMicros int64) (bool, error) {
		// On the caller's tx: create() holds the workspace lock and may hold the
		// pool's only connection.
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
			// No reported spend: record the Run, charge nothing.
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
			// Cleanup re-runs until the run is `cleaned`; one key keeps it one debit.
			IdempotencyKey: key,
		})
		return err
	}
}
