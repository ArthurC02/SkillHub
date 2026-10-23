package run

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

const auditReasonKey = "reason"

const (
	MinContinuationRounds  = 1
	MaxContinuationRounds  = 10
	maxSettingsReasonChars = limits.SetContinuationRoundsBodyNoteMaxLength
)

var (
	ErrSettingsReasonRequired = errors.New("run: a settings change needs a reason of at most 1000 characters")
	ErrRoundsOutOfRange       = errors.New("run: continuation rounds must be between 1 and 10")
)

type Settings struct {
	ContinuationRounds int
	Reason             string
	SetBy              pgtype.UUID
	SetAt              time.Time
	Changed            bool
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, err := s.queries().GetRunSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{ContinuationRounds: DefaultContinuationRounds}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return settingsOf(row), nil
}

func (s *Service) ContinuationRounds(ctx context.Context) (int, error) {
	settings, err := s.Settings(ctx)
	return settings.ContinuationRounds, err
}

func (s *Service) SetContinuationRounds(ctx context.Context, operator pgtype.UUID, rounds int, reason string) (Settings, error) {
	reason, err := settingsReason(reason)
	if err != nil {
		return Settings{}, err
	}
	if rounds < MinContinuationRounds || rounds > MaxContinuationRounds {
		return Settings{}, ErrRoundsOutOfRange
	}
	var updated Settings
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before := DefaultContinuationRounds
		row, err := q.GetRunSettings(ctx)
		switch {
		case err == nil:
			before = int(row.ContinuationRounds)
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		row, err = q.SetRunSettings(ctx, gen.SetRunSettingsParams{
			ContinuationRounds: int32(rounds), Reason: reason, SetBy: operator,
		})
		if err != nil {
			return err
		}
		updated = settingsOf(row)
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionRunSettingsSet, ResourceType: audit.ResourceRunSettings,
			Metadata: settingChange("continuation_rounds", before, rounds, reason),
		})
	})
	return updated, err
}

func settingsOf(row gen.RunSetting) Settings {
	return Settings{
		ContinuationRounds: int(row.ContinuationRounds), Reason: row.Reason,
		SetBy: row.SetBy, SetAt: row.SetAt.Time, Changed: true,
	}
}

const (
	MinRunBudgetUSD = 0.05
	MaxRunBudgetUSD = 5.0
	microsPerUSD    = 1_000_000
)

var (
	ErrRunBudgetOutOfRange = errors.New("run: the model spend ceiling of a run is outside the credits its band allows")
	ErrRunBudgetUnpriced   = errors.New("run: the model spend ceiling of a run cannot be priced in credits without a credit ledger")
)

type RunBudgetBand struct{ MinCredits, MaxCredits int64 }

func runBudgetBand(l CreditLedger) (RunBudgetBand, bool) {
	if l == nil {
		return RunBudgetBand{}, false
	}
	lo, okLo := l.CreditsForUSD(MinRunBudgetUSD)
	hi, okHi := l.CreditsWithinUSD(MaxRunBudgetUSD)
	return RunBudgetBand{MinCredits: lo, MaxCredits: hi}, okLo && okHi && lo <= hi
}

func usdForCeiling(l CreditLedger, credits int64) (float64, error) {
	band, ok := runBudgetBand(l)
	if !ok {
		return 0, ErrRunBudgetUnpriced
	}
	if credits < band.MinCredits || credits > band.MaxCredits {
		return 0, ErrRunBudgetOutOfRange
	}
	usd, ok := l.USDForCredits(credits)
	if !ok {
		return 0, ErrRunBudgetUnpriced
	}
	return math.Min(math.Max(usd, MinRunBudgetUSD), MaxRunBudgetUSD), nil
}

type RunBudgetSetting struct {
	USD        float64
	DefaultUSD float64
	Reason     string
	SetBy      pgtype.UUID
	SetAt      time.Time
	Overridden bool
}

func runBudgetIn(ctx context.Context, q *gen.Queries, d Deployment) (RunBudgetSetting, error) {
	setting := RunBudgetSetting{USD: d.Budget(), DefaultUSD: d.Budget()}
	row, err := q.GetRunModelBudgetSetting(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return setting, nil
	}
	if err != nil {
		return RunBudgetSetting{}, err
	}
	setting.USD = float64(row.BudgetMicros) / microsPerUSD
	setting.Reason, setting.SetBy, setting.SetAt, setting.Overridden = row.Reason, row.SetBy, row.SetAt.Time, true
	return setting, nil
}

func setRunBudget(ctx context.Context, s *Service, operator pgtype.UUID, credits int64, reason string) (RunBudgetSetting, error) {
	d := s.Deployment
	reason, err := settingsReason(reason)
	if err != nil {
		return RunBudgetSetting{}, err
	}
	usd, err := usdForCeiling(s.Ledger, credits)
	if err != nil {
		return RunBudgetSetting{}, err
	}
	var updated RunBudgetSetting
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := runBudgetIn(ctx, q, d)
		if err != nil {
			return err
		}
		if _, err := q.SetRunModelBudgetSetting(ctx, gen.SetRunModelBudgetSettingParams{
			BudgetMicros: int64(math.Round(usd * microsPerUSD)), Reason: reason, SetBy: operator,
		}); err != nil {
			return err
		}
		if updated, err = runBudgetIn(ctx, q, d); err != nil {
			return err
		}
		return auditRunBudget(ctx, tx, operator, budgetChange{from: before.USD, to: updated.USD}, reason)
	})
	return updated, err
}

func clearRunBudget(ctx context.Context, s *Service, operator pgtype.UUID, reason string) (RunBudgetSetting, error) {
	d := s.Deployment
	reason, err := settingsReason(reason)
	if err != nil {
		return RunBudgetSetting{}, err
	}
	var updated RunBudgetSetting
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := runBudgetIn(ctx, q, d)
		if err != nil {
			return err
		}
		if _, err := q.ClearRunModelBudgetSetting(ctx); err != nil {
			return err
		}
		updated = RunBudgetSetting{USD: d.Budget(), DefaultUSD: d.Budget()}
		return auditRunBudget(ctx, tx, operator, budgetChange{from: before.USD, to: updated.USD}, reason)
	})
	return updated, err
}

type budgetChange struct{ from, to float64 }

func auditRunBudget(ctx context.Context, tx pgx.Tx, operator pgtype.UUID, change budgetChange, reason string) error {
	return audit.Log(ctx, tx, audit.Event{
		Actor: operator, Action: audit.ActionRunSettingsSet, ResourceType: audit.ResourceRunSettings,
		Metadata: settingChange("model_budget_usd", change.from, change.to, reason),
	})
}

func settingsReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxSettingsReasonChars {
		return "", ErrSettingsReasonRequired
	}
	return reason, nil
}

const (
	MinRunTPMLimit = 10_000
	MaxRunTPMLimit = 2_000_000
)

var ErrRunRateLimitOutOfRange = errors.New("run: the tokens-per-minute limit of a run key must be between 10000 and 2000000")

type RunRateLimitSetting struct {
	Limit        int
	DefaultLimit int
	Reason       string
	SetBy        pgtype.UUID
	SetAt        time.Time
	Overridden   bool
}

func runRateLimitIn(ctx context.Context, q *gen.Queries, d Deployment) (RunRateLimitSetting, error) {
	setting := RunRateLimitSetting{Limit: d.TokensPerMinute(), DefaultLimit: d.TokensPerMinute()}
	row, err := q.GetRunRateLimitSetting(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return setting, nil
	}
	if err != nil {
		return RunRateLimitSetting{}, err
	}
	setting.Limit = int(row.TpmLimit)
	setting.Reason, setting.SetBy, setting.SetAt, setting.Overridden = row.Reason, row.SetBy, row.SetAt.Time, true
	return setting, nil
}

func setRunRateLimit(ctx context.Context, s *Service, operator pgtype.UUID, limit int, reason string) error {
	reason, err := settingsReason(reason)
	if err != nil {
		return err
	}
	if limit < MinRunTPMLimit || limit > MaxRunTPMLimit {
		return ErrRunRateLimitOutOfRange
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := runRateLimitIn(ctx, q, s.Deployment)
		if err != nil {
			return err
		}
		if _, err := q.SetRunRateLimitSetting(ctx, gen.SetRunRateLimitSettingParams{
			TpmLimit: int32(limit), Reason: reason, SetBy: operator,
		}); err != nil {
			return err
		}
		return auditRunRateLimit(ctx, tx, operator, [2]int{before.Limit, limit}, reason)
	})
}

func clearRunRateLimit(ctx context.Context, s *Service, operator pgtype.UUID, reason string) error {
	reason, err := settingsReason(reason)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := runRateLimitIn(ctx, q, s.Deployment)
		if err != nil {
			return err
		}
		if _, err := q.ClearRunRateLimitSetting(ctx); err != nil {
			return err
		}
		return auditRunRateLimit(ctx, tx, operator, [2]int{before.Limit, s.Deployment.TokensPerMinute()}, reason)
	})
}

func auditRunRateLimit(ctx context.Context, tx pgx.Tx, operator pgtype.UUID, fromTo [2]int, reason string) error {
	return audit.Log(ctx, tx, audit.Event{
		Actor: operator, Action: audit.ActionRunSettingsSet, ResourceType: audit.ResourceRunSettings,
		Metadata: settingChange("tokens_per_minute", fromTo[0], fromTo[1], reason),
	})
}

func settingChange(setting string, from, to any, reason string) map[string]any {
	return map[string]any{"setting": setting, "from": from, "to": to, auditReasonKey: reason}
}
