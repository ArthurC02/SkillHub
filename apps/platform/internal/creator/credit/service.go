package credit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

var (
	ErrUnavailable = errors.New("credit: capability unavailable")

	ErrInvalid = errors.New("credit: invalid input")

	ErrZeroAmount         = fmt.Errorf("%w: amount must not be zero", ErrInvalid)
	ErrReasonRequired     = fmt.Errorf("%w: reason is required", ErrInvalid)
	ErrGrantLowersBalance = fmt.Errorf("%w: only an adjustment may lower a balance", ErrInvalid)

	ErrBalanceOutOfRange = fmt.Errorf("%w: balance would fall below %d", ErrInvalid, MinBalanceCredits)

	ErrAccountGone = errors.New("credit: account not eligible")

	ErrGrantKeyReused = errors.New("credit: idempotency key already used for a different grant")
)

type AccountFacts struct {
	Exists bool
	Purged bool
}

type Service struct {
	Store  Store
	Config Config

	Facts func(ctx context.Context, db DBTX, userID pgtype.UUID) (AccountFacts, error)
}

func (s *Service) checkAccount(ctx context.Context, db DBTX, userID pgtype.UUID) error {
	if !userID.Valid || s.Facts == nil {
		return nil
	}
	f, err := s.Facts(ctx, db, userID)
	if err != nil {
		return err
	}
	if !f.Exists || f.Purged {
		return ErrAccountGone
	}
	return nil
}

const costRecordingWait = 10 * time.Second

func (s *Service) RecordCost(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error) {
	if s.Store == nil {
		return "", false, ErrUnavailable
	}
	if e.Kind == "" || e.IdempotencyKey == "" {
		return "", false, ErrInvalid
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), costRecordingWait)
	defer cancel()
	return s.Store.RecordCostEvent(recordCtx, tx, e)
}

func (s *Service) CostRecorded(ctx context.Context, idempotencyKey string) (bool, error) {
	if s.Store == nil {
		return false, ErrUnavailable
	}
	return s.Store.CostEventExists(ctx, nil, idempotencyKey)
}

type ChargeInput struct {
	Kind             CostKind
	Model            string
	PromptVersion    string
	PromptTokens     int64
	CompletionTokens int64

	UsdMicros *int64

	ReservedUsdMicros int64

	UserID         pgtype.UUID
	WorkspaceID    pgtype.UUID
	RefType        string
	RefID          pgtype.UUID
	IdempotencyKey string
}

type ChargeResult struct {
	CostEventID string

	Existed    bool
	Credits    int64
	NewBalance int64
	Estimated  bool
}

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
	if err := s.checkAccount(ctx, tx, in.UserID); err != nil {
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

		return ChargeResult{}, err
	}
	credits := CreditsForMicros(billed, s.Config.MicrosPerCredit)
	if estimated || credits == 0 {

		balance, err := s.Store.Balance(ctx, tx, in.UserID)
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

func (s *Service) CanAffordStep(ctx context.Context, userID pgtype.UUID, reservedUsdMicros int64) (bool, error) {
	return s.CanAffordStepIn(ctx, nil, userID, reservedUsdMicros)
}

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

		return false, err
	}
	reservedCredits := CreditsForMicros(billed, s.Config.MicrosPerCredit)
	return balance-reservedCredits >= s.Config.DebtFloorCredits, nil
}

type StartCheck struct {
	OK        bool
	Balance   int64
	Threshold int64

	Estimated bool
}

func (s *Service) CanStart(ctx context.Context, userID pgtype.UUID, statKind CostKind) (StartCheck, error) {
	if s.Store == nil {
		return StartCheck{}, ErrUnavailable
	}
	if err := s.checkAccount(ctx, nil, userID); err != nil {
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

func (s *Service) startThreshold(ctx context.Context, kind CostKind) (threshold int64, estimated bool, err error) {
	stats, err := s.Store.RecentStatistics(ctx, kind)
	if errors.Is(err, ErrNoStatistics) {
		return s.Config.StartFallbackCredits, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	threshold, estimated = s.thresholdFrom(stats)
	return threshold, estimated, nil
}

func (s *Service) thresholdFrom(stats Statistics) (threshold int64, estimated bool) {
	if stats.SampleCount < MinStatSamples || time.Since(stats.WindowEnd) > MaxStatisticsAge {
		return s.Config.StartFallbackCredits, true
	}
	if credits, billable := s.billableCredits(stats.P95UsdMicros); billable {
		return credits, false
	}
	return s.Config.StartFallbackCredits, true
}

func (s *Service) billableCredits(usdMicros int64) (int64, bool) {
	billed, err := BilledMicros(usdMicros, s.Config.MarkupBps)
	if err != nil {
		return 0, false
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), true
}

func (s *Service) Balance(ctx context.Context, userID pgtype.UUID) (int64, error) {
	if s.Store == nil {
		return 0, ErrUnavailable
	}
	return s.Store.Balance(ctx, nil, userID)
}

type Estimate struct {
	LowCredits       int64
	HighCredits      int64
	ThresholdCredits int64
	SampleCount      int

	Estimated bool
}

func (s *Service) Estimate(ctx context.Context, statKind CostKind) (Estimate, error) {
	if s.Store == nil {
		return Estimate{}, ErrUnavailable
	}
	stats, err := s.Store.RecentStatistics(ctx, statKind)
	if err != nil && !errors.Is(err, ErrNoStatistics) {
		return Estimate{}, err
	}
	threshold, estimated := s.thresholdFrom(stats)
	est := Estimate{
		LowCredits: threshold, HighCredits: threshold,
		ThresholdCredits: threshold, Estimated: estimated,
	}
	if estimated {
		return est, nil
	}
	est.SampleCount = stats.SampleCount
	if low, billable := s.billableCredits(stats.P50UsdMicros); billable {
		est.LowCredits = low
	}
	return est, nil
}

func (s *Service) CreditsForUSD(usd float64) (credits int64, ok bool) {
	micros, billable := BillableMicros(usd)
	if !billable {
		return 0, false
	}
	billed, err := BilledMicros(micros, s.Config.MarkupBps)
	if err != nil {
		return 0, false
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), true
}

// CreditsWithinUSD is the most credits whose worth does not exceed usd, for a ceiling shown as credits.
func (s *Service) CreditsWithinUSD(usd float64) (credits int64, ok bool) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 || s.Config.MicrosPerCredit <= 0 {
		return 0, false
	}
	scaled := math.Floor(usd * microsPerUSD)
	if scaled > float64(MaxBillableMicros) {
		return 0, false
	}
	micros := int64(scaled)
	return micros * s.Config.MarkupBps / basisPointsPerUnit / s.Config.MicrosPerCredit, true
}

// USDForCredits is what credits are worth to the platform, rounded down so a budget never exceeds its credits.
func (s *Service) USDForCredits(credits int64) (usd float64, ok bool) {
	if credits < 0 || s.Config.MarkupBps <= 0 || s.Config.MicrosPerCredit <= 0 ||
		credits > MaxBillableMicros/s.Config.MicrosPerCredit {
		return 0, false
	}
	micros := credits * s.Config.MicrosPerCredit * basisPointsPerUnit / s.Config.MarkupBps
	if micros > MaxBillableMicros {
		return 0, false
	}
	return float64(micros) / microsPerUSD, true
}

func OperatorEntryKind(credits int64) EntryKind {
	if credits < 0 {
		return EntryAdjustment
	}
	return EntryGrant
}

type GrantInput struct {
	UserID         pgtype.UUID
	WorkspaceID    pgtype.UUID
	EntryKind      EntryKind
	Credits        int64
	Reason         string
	OperatorID     pgtype.UUID
	IdempotencyKey string
}

func (s *Service) Grant(ctx context.Context, tx DBTX, in GrantInput) (int64, error) {
	if s.Store == nil {
		return 0, ErrUnavailable
	}
	switch in.EntryKind {
	case EntryGrant, EntryTopup, EntryAdjustment:
	default:
		return 0, ErrInvalid
	}
	in.Reason = strings.TrimSpace(in.Reason)
	switch {
	case in.Credits == 0:
		return 0, ErrZeroAmount
	case in.Reason == "":
		return 0, ErrReasonRequired
	case in.Credits < 0 && in.EntryKind != EntryAdjustment:
		return 0, ErrGrantLowersBalance
	case !in.UserID.Valid || !in.OperatorID.Valid || in.IdempotencyKey == "":
		return 0, ErrInvalid
	}
	if err := s.checkAccount(ctx, tx, in.UserID); err != nil {
		return 0, err
	}

	balance, applied, err := s.Store.ApplyGrant(ctx, tx, GrantEntry{
		UserID: in.UserID, EntryKind: in.EntryKind, Credits: in.Credits,
		Reason: in.Reason, OperatorID: in.OperatorID, IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return 0, err
	}
	if !applied {
		return balance, nil
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: in.OperatorID, Workspace: in.WorkspaceID, Action: audit.ActionCreditGrant,
		ResourceType: "credit_entry", ResourceID: in.UserID,
		Metadata: map[string]any{"kind": in.EntryKind, "credits": in.Credits, "reason": in.Reason},
	}); err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *Service) RecomputeStatistics(ctx context.Context, statKind CostKind, window time.Duration) (Statistics, error) {
	if s.Store == nil {
		return Statistics{}, ErrUnavailable
	}
	now := time.Now()
	var sweepErr error
	if statKind == KindCreationSession && s.Config.SessionIdle > 0 {
		if _, sweepErr = s.Store.SweepSessionSummaries(ctx, now.Add(-window), now.Add(-s.Config.SessionIdle)); sweepErr != nil {
			slog.Warn("credit: session summary sweep failed; recomputing from the summaries that exist", "error", sweepErr)
		}
	}
	stats, err := s.Store.RecomputeStatistics(ctx, statKind, now.Add(-window), now)
	if err != nil {
		return Statistics{}, errors.Join(sweepErr, err)
	}
	return stats, sweepErr
}

// SummarizeSession runs in a savepoint so a failed summary cannot abort the caller's transaction.
func (s *Service) SummarizeSession(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error {
	if s.Store == nil {
		return ErrUnavailable
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := s.Store.SummarizeSession(ctx, sp, sessionID); err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}

type Ledger struct {
	Balance int64
	Entries []LedgerEntry
}

const ledgerEntryLimit = 50

type LedgerQuery struct {
	Account   pgtype.UUID
	Workspace pgtype.UUID
	Operator  pgtype.UUID
}

func (s *Service) Ledger(ctx context.Context, tx DBTX, query LedgerQuery) (Ledger, error) {
	if s.Store == nil {
		return Ledger{}, ErrUnavailable
	}
	balance, err := s.Store.Balance(ctx, tx, query.Account)
	if err != nil {
		return Ledger{}, err
	}
	entries, err := s.Store.RecentEntries(ctx, tx, query.Account, ledgerEntryLimit)
	if err != nil {
		return Ledger{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: query.Operator, Workspace: query.Workspace, Action: audit.ActionCreditLookup,
		ResourceType: audit.ResourceCreditAccount, ResourceID: query.Account,
	}); err != nil {
		return Ledger{}, err
	}
	return Ledger{Balance: balance, Entries: entries}, nil
}

const StatementPageSize = 50

func (s *Service) Statement(ctx context.Context, userID pgtype.UUID, beforeAt time.Time, beforeID pgtype.UUID) ([]StatementEntry, bool, error) {
	if s.Store == nil {
		return nil, false, ErrUnavailable
	}
	entries, err := s.Store.OwnEntries(ctx, userID, EntryPage{
		BeforeAt: beforeAt, BeforeID: beforeID, Limit: StatementPageSize + 1,
	})
	if err != nil {
		return nil, false, err
	}
	if len(entries) > StatementPageSize {
		return entries[:StatementPageSize], true, nil
	}
	return entries, false, nil
}

func (s *Service) LatestStatistics(ctx context.Context) ([]KindStatistics, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	return s.Store.LatestStatistics(ctx)
}

func (s *Service) DailyCost(ctx context.Context, since time.Time) ([]DailyAmount, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	return s.Store.DailyCost(ctx, since)
}

func (s *Service) DailyCredits(ctx context.Context, since time.Time) ([]DailyAmount, int64, error) {
	if s.Store == nil {
		return nil, 0, ErrUnavailable
	}
	days, err := s.Store.DailyCredits(ctx, since)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Store.BalanceTotal(ctx)
	return days, total, err
}
