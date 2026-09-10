package credit

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

var (
	// ErrUnavailable: a required dependency (Store, or a Config that was
	// never loaded) is not wired. Fail closed rather than proceed with a
	// zero-value Config that would silently charge nothing.
	ErrUnavailable = errors.New("credit: capability unavailable")
	// ErrInvalid: the caller's input does not meet this method's contract
	// (missing idempotency key, non-positive credits, empty reason, ...).
	ErrInvalid = errors.New("credit: invalid input")
	// ErrAccountGone: Facts said this user no longer exists or has been
	// purged. Grants and charges both refuse it (decision 11).
	ErrAccountGone = errors.New("credit: account not eligible")
)

// auditActionGrant is a local audit action string (foundation/observability/
// audit's Action field is a plain string, not a closed enum owned by that
// package — see its Event.Action doc comment).
const auditActionGrant = "credit.grant"

// AccountFacts is what credit needs to know about a user before granting or
// charging their account. Injected by each composition root the same way
// skill/discovery injects SkillFacts and skill/admission injects
// VersionFacts (ADR-032 §1's Facts convention, ADR-068's "credit 本身不 import
// identity"): credit never imports the identity package itself.
//
// Keyed by user, not workspace: migration 0060's credit_accounts is keyed on
// user_id ("每個帳號" is the user — MVP's 1:1 user/workspace does not change
// that, per the migration's own comment), even though ADR-068's prose
// sometimes says "workspace" loosely. A workspace-scoped caller (creation,
// in particular) resolves its workspace to the owning user id before
// calling anything in this package — see this batch's report.
type AccountFacts struct {
	Exists bool
	Purged bool
}

// Service is credit's one entry point for every context that spends or
// grants credit. Every method fails closed (ErrUnavailable) when Store is
// nil, so an unwired Service can never be mistaken for "nothing to charge".
type Service struct {
	Store  Store
	Config Config
	// Facts answers AccountFacts for a user id. nil skips the check —
	// acceptable only for a composition root that has not wired identity
	// yet; any composition root that has MUST inject this (see doc.go).
	Facts func(ctx context.Context, userID pgtype.UUID) (AccountFacts, error)
}

func (s *Service) checkAccount(ctx context.Context, userID pgtype.UUID) error {
	if !userID.Valid || s.Facts == nil {
		return nil
	}
	f, err := s.Facts(ctx, userID)
	if err != nil {
		return err
	}
	if !f.Exists || f.Purged {
		return ErrAccountGone
	}
	return nil
}

// RecordCost writes a cost_events row (via Store.RecordCostEvent) without
// applying any debit. This is catalog's shape (ADR-032 §1 appendix A:
// "catalog → credit（搜尋 embedding、索引增強寫成本事件；MVP 不對這兩者扣點，
// 只寫入不查詢門檻）") — a call that costs the platform real money but is not
// (yet, MVP) billed to any one account.
func (s *Service) RecordCost(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error) {
	if s.Store == nil {
		return "", false, ErrUnavailable
	}
	if e.Kind == "" || e.IdempotencyKey == "" {
		return "", false, ErrInvalid
	}
	return s.Store.RecordCostEvent(ctx, tx, e)
}

// ChargeInput is one call's worth of real spend to charge to a specific
// user's account.
type ChargeInput struct {
	Kind             string
	Model            string
	PromptVersion    string
	PromptTokens     int64
	CompletionTokens int64
	// UsdMicros is the actual cost, in USD-micros. nil means the actual cost
	// is unknown (a gateway call whose usage report never came back);
	// Charge then bills ReservedUsdMicros instead and marks the entry
	// Estimated — it never charges zero for an unknown cost (ADR-068
	// decision 5, the same rule creation's own UsageUnknown flag exists
	// for).
	UsdMicros *int64
	// ReservedUsdMicros is the pre-call reserved estimate: used as the bill
	// whenever UsdMicros is nil, and by CanAffordStep's gate ② check before
	// the call is made at all.
	ReservedUsdMicros int64
	// UserID is the account debited — required (credit_entries.user_id is
	// NOT NULL). WorkspaceID is attribution only, and may be left invalid.
	UserID         pgtype.UUID
	WorkspaceID    pgtype.UUID
	RefType        string // "", RefCreationSession, RefRun or RefSkillVersion
	RefID          pgtype.UUID
	IdempotencyKey string
}

// ChargeResult is what a Charge call settled on — including on a replay,
// where it reports the original charge rather than charging again.
type ChargeResult struct {
	CostEventID string
	// Existed is true when this call was a replay of an already-recorded
	// IdempotencyKey: nothing new was written, Credits and NewBalance
	// describe what the first call already did.
	Existed    bool
	Credits    int64
	NewBalance int64
	Estimated  bool
}

// Charge is ADR-068 decision 5: record the real cost event, then debit the
// user's account for it after markup, inside tx — the caller's transaction,
// so this commits atomically with whatever else that transaction is doing
// (creation's AdvanceCreationSession, in particular). Idempotent on
// IdempotencyKey: calling Charge twice with the same key (a Worker retry
// settling the same session/revision) debits once.
func (s *Service) Charge(ctx context.Context, tx DBTX, in ChargeInput) (ChargeResult, error) {
	if s.Store == nil {
		return ChargeResult{}, ErrUnavailable
	}
	if in.Kind == "" || in.IdempotencyKey == "" || !in.UserID.Valid {
		return ChargeResult{}, ErrInvalid
	}
	estimated := in.UsdMicros == nil
	billingMicros := in.ReservedUsdMicros
	if !estimated {
		billingMicros = *in.UsdMicros
	}
	if billingMicros < 0 {
		return ChargeResult{}, ErrInvalid
	}
	if err := s.checkAccount(ctx, in.UserID); err != nil {
		return ChargeResult{}, err
	}

	eventID, eventExisted, err := s.Store.RecordCostEvent(ctx, tx, CostEvent{
		Kind: in.Kind, Model: in.Model, PromptVersion: in.PromptVersion,
		PromptTokens: in.PromptTokens, CompletionTokens: in.CompletionTokens,
		UsdMicros: billingMicros, Estimated: estimated,
		WorkspaceID: in.WorkspaceID, UserID: in.UserID,
		RefType: in.RefType, RefID: in.RefID, IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return ChargeResult{}, err
	}

	billed, err := BilledMicros(billingMicros, s.Config.MarkupBps)
	if err != nil {
		// An amount out of range is a bug upstream, not a free call: refusing
		// here leaves the cost event written and the debit absent, which is a
		// visible inconsistency, where charging zero would be an invisible one.
		return ChargeResult{}, err
	}
	credits := CreditsForMicros(billed, s.Config.MicrosPerCredit)
	if credits == 0 {
		// A call that cost the platform nothing at all. The cost event above is
		// still written — it happened, and the statistics window should see it —
		// but no debit is: migration 0060 requires a debit to move the balance
		// (delta_credits < 0), so a zero debit is not a row this schema has, and
		// writing one credit for a free call would be inventing a charge.
		//
		// This is not decision 5's "never charge zero for an unknown cost": that
		// case has UsdMicros nil, bills the reservation, and lands above with a
		// non-zero amount. This one is a genuinely zero measured cost.
		balance, err := s.Store.Balance(ctx, nil, in.UserID)
		if err != nil {
			return ChargeResult{}, err
		}
		return ChargeResult{CostEventID: eventID, Existed: eventExisted, NewBalance: balance, Estimated: estimated}, nil
	}
	balance, debitExisted, err := s.Store.ApplyDebit(ctx, tx, DebitEntry{
		CostEventID: eventID, UserID: in.UserID, Credits: credits,
		UsdMicros: billingMicros,
		MarkupBps: s.Config.MarkupBps, Estimated: estimated,
		RefType: in.RefType, RefID: in.RefID, IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return ChargeResult{}, err
	}
	return ChargeResult{
		CostEventID: eventID,
		Existed:     eventExisted || debitExisted,
		Credits:     credits,
		NewBalance:  balance,
		Estimated:   estimated,
	}, nil
}

// CanAffordStep is gate ② (ADR-068 decision 7): checked right before a paid
// call, with that step's reserved cost. false means charging this step
// would put the balance below the -50 floor; the step must not be made.
// This never writes anything — the charge itself always happens afterward
// through Charge, floor or no floor, because the cost was already incurred
// (decision 7: "已發生的成本仍照決策 5 結算").
func (s *Service) CanAffordStep(ctx context.Context, userID pgtype.UUID, reservedUsdMicros int64) (bool, error) {
	return s.CanAffordStepIn(ctx, nil, userID, reservedUsdMicros)
}

// CanAffordStepIn is CanAffordStep read on the caller's transaction, so a gate
// already holding a pool's only connection does not wait for a second one.
func (s *Service) CanAffordStepIn(ctx context.Context, tx DBTX, userID pgtype.UUID, reservedUsdMicros int64) (bool, error) {
	if s.Store == nil {
		return false, ErrUnavailable
	}
	balance, err := s.Store.Balance(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	billed, err := BilledMicros(reservedUsdMicros, s.Config.MarkupBps)
	if err != nil {
		// Fail closed: a reservation this package cannot price is not a step
		// this package may wave through.
		return false, err
	}
	reservedCredits := CreditsForMicros(billed, s.Config.MicrosPerCredit)
	return balance-reservedCredits >= s.Config.DebtFloorCredits, nil
}

// StartCheck is gate ①'s answer: whether a new session/call of some kind
// may begin, and what the check was based on.
type StartCheck struct {
	OK        bool
	Balance   int64
	Threshold int64
	// Estimated is true when Threshold came from Config.StartFallbackCredits
	// rather than measured statistics (fewer than MinStatSamples samples).
	Estimated bool
}

// CanStart is gate ① (ADR-068 decision 7 & 8): checked before a new session
// or one-shot generation may begin at all. The threshold is the most recent
// window's p95 cost for statKind, marked up and rounded up like any other
// conversion, or Config.StartFallbackCredits when there are not yet
// MinStatSamples samples for statKind.
func (s *Service) CanStart(ctx context.Context, userID pgtype.UUID, statKind string) (StartCheck, error) {
	if s.Store == nil {
		return StartCheck{}, ErrUnavailable
	}
	if err := s.checkAccount(ctx, userID); err != nil {
		return StartCheck{}, err
	}
	balance, err := s.Store.Balance(ctx, nil, userID)
	if err != nil {
		return StartCheck{}, err
	}
	threshold, estimated, err := s.startThreshold(ctx, statKind)
	if err != nil {
		return StartCheck{}, err
	}
	return StartCheck{OK: balance >= threshold, Balance: balance, Threshold: threshold, Estimated: estimated}, nil
}

func (s *Service) startThreshold(ctx context.Context, kind string) (threshold int64, estimated bool, err error) {
	stats, err := s.Store.RecentStatistics(ctx, kind)
	if errors.Is(err, ErrNoStatistics) {
		return s.Config.StartFallbackCredits, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	if stats.SampleCount < MinStatSamples {
		return s.Config.StartFallbackCredits, true, nil
	}
	billed, err := BilledMicros(stats.P95UsdMicros, s.Config.MarkupBps)
	if err != nil {
		// A p95 out of range means the statistics themselves are wrong; the
		// conservative constant is the honest answer, flagged as an estimate.
		return s.Config.StartFallbackCredits, true, nil
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), false, nil
}

// Balance is the account's materialized balance — the column decision 4
// keeps in step with the entries inside their own transaction, never a live
// re-sum of credit_entries. Reconciling the two is a maintenance-time job;
// this is the request-time read.
func (s *Service) Balance(ctx context.Context, userID pgtype.UUID) (int64, error) {
	if s.Store == nil {
		return 0, ErrUnavailable
	}
	return s.Store.Balance(ctx, nil, userID)
}

// Estimate is what one call of statKind is expected to cost, in credits, and
// the threshold gate ① measures a balance against. It is the display side of
// [Service.CanStart] — same threshold, same fallback, same rounding — so a
// screen and the gate that blocks it can never quote different numbers,
// which is the failure entitlements.QuotaView was built to avoid for
// PDM-010's counters.
//
// The band is p50 to p95: the middle of what these calls actually cost, and
// the point gate ① draws its line at. Both are marked up and rounded up like
// every other conversion in this package.
type Estimate struct {
	LowCredits       int64
	HighCredits      int64
	ThresholdCredits int64
	SampleCount      int
	// Estimated is true when the threshold came from
	// Config.StartFallbackCredits rather than a measured p95.
	Estimated bool
}

// Estimate reports the current window for statKind. With fewer than
// MinStatSamples samples the whole band collapses to the conservative
// constant and Estimated is true — a fallback that presented itself as a
// measurement would be worse than no number at all.
func (s *Service) Estimate(ctx context.Context, statKind string) (Estimate, error) {
	if s.Store == nil {
		return Estimate{}, ErrUnavailable
	}
	threshold, estimated, err := s.startThreshold(ctx, statKind)
	if err != nil {
		return Estimate{}, err
	}
	est := Estimate{
		LowCredits: threshold, HighCredits: threshold,
		ThresholdCredits: threshold, Estimated: estimated,
	}
	if estimated {
		return est, nil
	}
	stats, err := s.Store.RecentStatistics(ctx, statKind)
	if err != nil {
		// The threshold above already read these statistics successfully, so
		// a failure here is a race with the recompute job, not a missing
		// window: report the threshold band rather than an error a screen
		// would have to render as a failure.
		return est, nil
	}
	est.SampleCount = stats.SampleCount
	if low, err := BilledMicros(stats.P50UsdMicros, s.Config.MarkupBps); err == nil {
		est.LowCredits = CreditsForMicros(low, s.Config.MicrosPerCredit)
	}
	return est, nil
}

// CreditsForUSD converts a dollar figure into the number of credits the
// platform shows for it. It is the one place a US dollar becomes a Credit on
// a screen (ADR-068 decision 1: 「平台對使用者的每一個成本呈現，只有一種單位」),
// and the composition roots hand it to the contexts that may not import this
// package at all — trial/execution's pre-run estimate and trial/evidence's
// trace usage, both of which depguard denies a credit import.
//
// The markup is applied, and that is the whole reason this is not a unit
// relabelling. A displayed credit figure has to mean the same thing as a
// charged one, or the number on the run page and the number that left the
// balance are two different quantities wearing one word. Rounded up, like
// every other conversion here.
//
// A cost the platform cannot represent — negative, non-finite, or past the
// billable ceiling — converts to 0, false. false means "no number", not
// "free": the caller renders absence, which is what every one of these
// surfaces already does for an unreported gateway cost.
func (s *Service) CreditsForUSD(usd float64) (credits int64, ok bool) {
	micros, estimated := UsageCost(&usd, "gateway")
	if estimated {
		// UsageCost only reports estimated here when the value itself was
		// unusable: the source is pinned to "gateway" one line above.
		return 0, false
	}
	billed, err := BilledMicros(micros, s.Config.MarkupBps)
	if err != nil {
		return 0, false
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), true
}

// GrantInput is one operator-initiated balance change (decision 10). A
// downward Adjustment is the only kind allowed negative Credits.
type GrantInput struct {
	UserID         pgtype.UUID
	EntryKind      string // EntryGrant, EntryTopup or EntryAdjustment
	Credits        int64
	Reason         string // SEC-011: required
	OperatorID     pgtype.UUID
	IdempotencyKey string
}

// Grant records one operator-initiated entry: grant, topup (MVP: same
// mechanism), or a manual adjustment — SEC-011's "必填理由、audit event、不可
// 自助授予" all apply, so a missing reason or operator id is refused before
// anything is written, and the audit event lands in the same transaction as
// the balance change.
func (s *Service) Grant(ctx context.Context, tx DBTX, in GrantInput) (int64, error) {
	if s.Store == nil {
		return 0, ErrUnavailable
	}
	switch in.EntryKind {
	case EntryGrant, EntryTopup, EntryAdjustment:
	default:
		return 0, ErrInvalid
	}
	if !in.UserID.Valid || !in.OperatorID.Valid || in.Credits == 0 ||
		strings.TrimSpace(in.Reason) == "" || in.IdempotencyKey == "" {
		return 0, ErrInvalid
	}
	if err := s.checkAccount(ctx, in.UserID); err != nil {
		return 0, err
	}
	// GrantInput and GrantEntry are field-for-field the same today: one is what
	// a caller says, the other what the store writes. The conversion keeps them
	// separable, so a future input field that is not a stored column does not
	// have to break this line.
	balance, err := s.Store.ApplyGrant(ctx, tx, GrantEntry(in))
	if err != nil {
		return 0, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: in.OperatorID, Action: auditActionGrant,
		ResourceType: "credit_entry", ResourceID: in.UserID,
		Metadata: map[string]any{"kind": in.EntryKind, "credits": in.Credits, "reason": in.Reason},
	}); err != nil {
		return 0, err
	}
	return balance, nil
}

// RecomputeStatistics is ADR-068 decision 9: aggregates statKind's cost
// events over [now-window, now) and writes the result as the newest
// cost_statistics row (Store.RecomputeStatistics — one Postgres statement
// pair, see store.go). The same call serves both of decision 9's triggers:
// a composition root calls this on a fixed daily schedule (job.go's
// RecomputeWorker) and, optionally, right after a session ends, with a
// shorter or longer window as it sees fit — this package does not
// distinguish the two, only computes what it is asked to.
func (s *Service) RecomputeStatistics(ctx context.Context, statKind string, window time.Duration) (Statistics, error) {
	if s.Store == nil {
		return Statistics{}, ErrUnavailable
	}
	now := time.Now()
	return s.Store.RecomputeStatistics(ctx, statKind, now.Add(-window), now)
}

// PurgeUser deletes or de-identifies one user's credit rows (decision 11).
// Unlike workspace/purge.go's WorkspacePurge (which every other context's
// purge step matches), this takes a userID: credit_accounts and
// credit_entries are keyed on the user, not on any one of their workspaces.
// See this batch's report for the workspace->user resolution the
// composition root needs before it can slot this into identity's
// per-workspace purgeSteps() list.
func (s *Service) PurgeUser(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) error {
	if s.Store == nil {
		return ErrUnavailable
	}
	return s.Store.PurgeUser(ctx, tx, userID)
}
