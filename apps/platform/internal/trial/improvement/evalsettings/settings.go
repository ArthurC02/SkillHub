package evalsettings

import (
	"context"
	"errors"
	"maps"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

const maxSettingsReasonChars = limits.SetJudgePanelBodyNoteMaxLength

type Service struct {
	Pool *pgxpool.Pool
}

var ErrSettingsReasonRequired = errors.New("eval: a settings change needs a reason of at most 1000 characters")

var ErrInjectionCheckPrice = errors.New("eval: the injection check needs a price between the contract's bounds")

const (
	minInjectionCheckCredits = 1
	maxInjectionCheckCredits = limits.SetInjectionCheckBodyCreditsPerMillionTokensMaximum
)

type Settings struct {
	JudgePanel bool
	Reason     string
	SetBy      pgtype.UUID
	SetAt      time.Time
	Changed    bool

	InjectionCheck       InjectionCheck
	ReflectionCandidates ReflectionCandidates
}

type InjectionCheck struct {
	Enabled                 bool
	CreditsPerMillionTokens int64
	Reason                  string
	SetAt                   time.Time
	Changed                 bool
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, err := gen.New(s.Pool).GetEvaluationSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{ReflectionCandidates: ReflectionCandidates{Count: 1}}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return settingsOf(row), nil
}

func (s *Service) JudgePanelEnabled(ctx context.Context) (bool, error) {
	settings, err := s.Settings(ctx)
	return settings.JudgePanel, err
}

func (s *Service) SetJudgePanel(ctx context.Context, operator pgtype.UUID, on bool, reason string) (Settings, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxSettingsReasonChars {
		return Settings{}, ErrSettingsReasonRequired
	}
	var updated Settings
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := q.GetEvaluationSettings(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err := q.SetEvaluationSettings(ctx, gen.SetEvaluationSettingsParams{JudgePanel: on, Reason: &reason, SetBy: operator})
		if err != nil {
			return err
		}
		updated = settingsOf(row)
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionEvaluationSettingsSet, ResourceType: audit.ResourceEvaluationSettings,
			Metadata: settingChanged("judge_panel", before.JudgePanel, on, reason, nil),
		})
	})
	return updated, err
}

func (s *Service) InjectionCheckPolicy(ctx context.Context) (on bool, creditsPerMillionTokens int64, err error) {
	settings, err := s.Settings(ctx)
	return settings.InjectionCheck.Enabled, settings.InjectionCheck.CreditsPerMillionTokens, err
}

func (s *Service) SetInjectionCheck(ctx context.Context, operator pgtype.UUID, on bool, creditsPerMillionTokens int64, reason string) (Settings, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxSettingsReasonChars {
		return Settings{}, ErrSettingsReasonRequired
	}
	if creditsPerMillionTokens < minInjectionCheckCredits || creditsPerMillionTokens > maxInjectionCheckCredits {
		return Settings{}, ErrInjectionCheckPrice
	}
	var updated Settings
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := q.GetEvaluationSettings(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		price := int32(creditsPerMillionTokens)
		row, err := q.SetEvaluationInjectionCheck(ctx, gen.SetEvaluationInjectionCheckParams{
			InjectionCheck: on, CreditsPerMillionTokens: &price, Reason: &reason,
		})
		if err != nil {
			return err
		}
		updated = settingsOf(row)
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionEvaluationSettingsSet, ResourceType: audit.ResourceEvaluationSettings,
			Metadata: settingChanged("injection_check", before.InjectionCheck, on, reason, map[string]any{"credits_per_million_tokens": creditsPerMillionTokens}),
		})
	})
	return updated, err
}

func settingsOf(row gen.EvaluationSetting) Settings {
	settings := Settings{JudgePanel: row.JudgePanel, SetBy: row.SetBy, Changed: row.Reason != nil}
	if settings.Changed {
		settings.Reason, settings.SetAt = *row.Reason, row.SetAt.Time
	}
	check := InjectionCheck{Enabled: row.InjectionCheck, Changed: row.InjectionCheckReason != nil}
	if row.InjectionCheckCreditsPerMillionTokens != nil {
		check.CreditsPerMillionTokens = int64(*row.InjectionCheckCreditsPerMillionTokens)
	}
	if check.Changed {
		check.Reason, check.SetAt = *row.InjectionCheckReason, row.InjectionCheckSetAt.Time
	}
	settings.InjectionCheck = check
	settings.ReflectionCandidates = reflectionCandidatesOf(row)
	return settings
}

const MaxReflectionCandidates = 4

var ErrReflectionCandidates = errors.New("eval: a round asks for 1 to 4 candidates and its credit cap, when set, is positive")

type ReflectionCandidates struct {
	Count          int
	RoundCreditCap *int64
	Reason         string
	SetAt          time.Time
	Changed        bool
}

func (s *Service) ReflectionPolicy(ctx context.Context) (ReflectionCandidates, error) {
	settings, err := s.Settings(ctx)
	return settings.ReflectionCandidates, err
}

func (s *Service) SetReflectionCandidates(
	ctx context.Context, operator pgtype.UUID, count int, roundCreditCap *int64, reason string,
) (Settings, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxSettingsReasonChars {
		return Settings{}, ErrSettingsReasonRequired
	}
	if count < 1 || count > MaxReflectionCandidates || (roundCreditCap != nil && (*roundCreditCap < 1 || *roundCreditCap > math.MaxInt32)) {
		return Settings{}, ErrReflectionCandidates
	}
	var updated Settings
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := q.GetEvaluationSettings(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var cap *int32
		if roundCreditCap != nil {
			c := int32(*roundCreditCap)
			cap = &c
		}
		row, err := q.SetEvaluationReflectionCandidates(ctx, gen.SetEvaluationReflectionCandidatesParams{
			ReflectionCandidates: int16(count), ReflectionRoundCreditCap: cap, Reason: &reason,
		})
		if err != nil {
			return err
		}
		updated = settingsOf(row)
		previous := settingsOf(before).ReflectionCandidates
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionEvaluationSettingsSet, ResourceType: audit.ResourceEvaluationSettings,
			Metadata: settingChanged("reflection_candidates", previous.Count, count, reason, map[string]any{
				"round_credit_cap_from": previous.RoundCreditCap, "round_credit_cap_to": roundCreditCap,
			}),
		})
	})
	return updated, err
}

func reflectionCandidatesOf(row gen.EvaluationSetting) ReflectionCandidates {
	out := ReflectionCandidates{Count: max(int(row.ReflectionCandidates), 1), Changed: row.ReflectionCandidatesReason != nil}
	if row.ReflectionRoundCreditCap != nil {
		cap := int64(*row.ReflectionRoundCreditCap)
		out.RoundCreditCap = &cap
	}
	if out.Changed {
		out.Reason, out.SetAt = *row.ReflectionCandidatesReason, row.ReflectionCandidatesSetAt.Time
	}
	return out
}

func settingChanged(setting string, from, to any, reason string, details map[string]any) map[string]any {
	out := map[string]any{"setting": setting, "from": from, "to": to, "reason": reason}
	maps.Copy(out, details)
	return out
}
