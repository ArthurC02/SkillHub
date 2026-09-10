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
	ErrUnavailable = errors.New("credit: capability unavailable")

	ErrInvalid = errors.New("credit: invalid input")

	ErrAccountGone = errors.New("credit: account not eligible")
)

const auditActionGrant = "credit.grant"

type AccountFacts struct {
	Exists bool
	Purged bool
}

type Service struct {
	Store  Store
	Config Config

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

func (s *Service) RecordCost(ctx context.Context, tx DBTX, e CostEvent) (id string, existed bool, err error) {
	if s.Store == nil {
		return "", false, ErrUnavailable
	}
	if e.Kind == "" || e.IdempotencyKey == "" {
		return "", false, ErrInvalid
	}
	return s.Store.RecordCostEvent(ctx, tx, e)
}

type ChargeInput struct {
	Kind             string
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

		return ChargeResult{}, err
	}
	credits := CreditsForMicros(billed, s.Config.MicrosPerCredit)
	if credits == 0 {

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

		return s.Config.StartFallbackCredits, true, nil
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), false, nil
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

		return est, nil
	}
	est.SampleCount = stats.SampleCount
	if low, err := BilledMicros(stats.P50UsdMicros, s.Config.MarkupBps); err == nil {
		est.LowCredits = CreditsForMicros(low, s.Config.MicrosPerCredit)
	}
	return est, nil
}

func (s *Service) CreditsForUSD(usd float64) (credits int64, ok bool) {
	micros, estimated := UsageCost(&usd, "gateway")
	if estimated {

		return 0, false
	}
	billed, err := BilledMicros(micros, s.Config.MarkupBps)
	if err != nil {
		return 0, false
	}
	return CreditsForMicros(billed, s.Config.MicrosPerCredit), true
}

type GrantInput struct {
	UserID         pgtype.UUID
	EntryKind      string
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
	if !in.UserID.Valid || !in.OperatorID.Valid || in.Credits == 0 ||
		strings.TrimSpace(in.Reason) == "" || in.IdempotencyKey == "" {
		return 0, ErrInvalid
	}
	if err := s.checkAccount(ctx, in.UserID); err != nil {
		return 0, err
	}

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

func (s *Service) RecomputeStatistics(ctx context.Context, statKind string, window time.Duration) (Statistics, error) {
	if s.Store == nil {
		return Statistics{}, ErrUnavailable
	}
	now := time.Now()
	return s.Store.RecomputeStatistics(ctx, statKind, now.Add(-window), now)
}

func (s *Service) PurgeUser(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) error {
	if s.Store == nil {
		return ErrUnavailable
	}
	return s.Store.PurgeUser(ctx, tx, userID)
}
