package eval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const maxSettingsReasonBytes = 1000

var ErrSettingsReasonRequired = errors.New("eval: a settings change needs a reason of at most 1000 bytes")

type Settings struct {
	JudgePanel bool
	Reason     string
	SetBy      pgtype.UUID
	SetAt      time.Time
	Changed    bool
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
	if reason == "" || len(reason) > maxSettingsReasonBytes {
		return Settings{}, ErrSettingsReasonRequired
	}
	var updated Settings
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := q.GetEvaluationSettings(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err := q.SetEvaluationSettings(ctx, gen.SetEvaluationSettingsParams{JudgePanel: on, Reason: reason, SetBy: operator})
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

func settingsOf(row gen.EvaluationSetting) Settings {
	return Settings{JudgePanel: row.JudgePanel, Reason: row.Reason, SetBy: row.SetBy, SetAt: row.SetAt.Time, Changed: true}
}
