package run

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

const auditReasonKey = "reason"

const (
	MinContinuationRounds  = 1
	MaxContinuationRounds  = 10
	maxSettingsReasonBytes = 1000
)

var (
	ErrSettingsReasonRequired = errors.New("run: a settings change needs a reason of at most 1000 bytes")
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
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > maxSettingsReasonBytes {
		return Settings{}, ErrSettingsReasonRequired
	}
	if rounds < MinContinuationRounds || rounds > MaxContinuationRounds {
		return Settings{}, ErrRoundsOutOfRange
	}
	var updated Settings
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
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
			Metadata: map[string]any{"setting": "continuation_rounds", "from": before, "to": rounds, auditReasonKey: reason},
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
