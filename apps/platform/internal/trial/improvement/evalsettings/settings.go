package evalsettings

import (
	"context"
	"errors"
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

	InjectionCheck InjectionCheck
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
		return Settings{}, nil
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
			Metadata: map[string]any{"setting": "judge_panel", "from": before.JudgePanel, "to": on, "reason": reason},
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
			Metadata: map[string]any{
				"setting": "injection_check", "from": before.InjectionCheck, "to": on,
				"credits_per_million_tokens": creditsPerMillionTokens, "reason": reason,
			},
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
	return settings
}
