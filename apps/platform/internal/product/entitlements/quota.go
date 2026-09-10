package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrQuotaExceeded = errors.New("this workspace has used its free run allowance")

var ErrAllowanceUnavailable = errors.New("the allowance could not be counted")

const (
	quotaFirstWindowRuns = 20
	quotaWindowRuns      = 30
	quotaDailyRuns       = 5
	quotaWindowDays      = 30
)

type QuotaLimits struct {
	Daily int

	Window int

	FirstWindow int

	WindowDays int
}

func DefaultQuotaLimits() QuotaLimits {
	return QuotaLimits{
		Daily:       quotaDailyRuns,
		Window:      quotaWindowRuns,
		FirstWindow: quotaFirstWindowRuns,
		WindowDays:  quotaWindowDays,
	}
}

func (l QuotaLimits) Enforced() bool { return l.WindowDays > 0 && (l.Daily > 0 || l.Window > 0) }

type QuotaState struct {
	RemainingToday  int
	RemainingWindow int
	WindowResetsAt  time.Time
	Limits          QuotaReport
}

type QuotaView struct {
	RemainingToday  int         `json:"remaining_today"`
	RemainingWindow int         `json:"remaining_window"`
	WindowResetsAt  string      `json:"window_resets_at"`
	Limits          QuotaReport `json:"limits"`
}

func (s QuotaState) View() QuotaView {
	return QuotaView{
		RemainingToday:  s.RemainingToday,
		RemainingWindow: s.RemainingWindow,
		WindowResetsAt:  s.WindowResetsAt.UTC().Format(time.RFC3339),
		Limits:          s.Limits,
	}
}

type QuotaReport struct {
	Daily      int `json:"daily"`
	Window     int `json:"window"`
	WindowDays int `json:"window_days"`
	Concurrent int `json:"concurrent"`
}

type RunUsage struct {
	Used   int64
	Oldest *time.Time
}

type UsageReader struct {
	WorkspaceCreatedAt func(context.Context, pgtype.UUID) (time.Time, error)
	CountRuns          func(context.Context, pgtype.UUID, time.Time) (RunUsage, error)
}

func Usage(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID, now time.Time,
) (QuotaState, error) {
	if reader.WorkspaceCreatedAt == nil || reader.CountRuns == nil {
		return QuotaState{}, errors.New("policy: quota usage readers not injected")
	}
	created, err := reader.WorkspaceCreatedAt(ctx, workspaceID)
	if err != nil {
		return QuotaState{}, err
	}
	windowStart := now.Add(-time.Duration(l.WindowDays) * 24 * time.Hour)

	window, err := reader.CountRuns(ctx, workspaceID, windowStart)
	if err != nil {
		return QuotaState{}, err
	}
	day, err := reader.CountRuns(ctx, workspaceID, now.Add(-24*time.Hour))
	if err != nil {
		return QuotaState{}, err
	}

	windowLimit := l.Window
	firstWindowEnds := created.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
	inFirstWindow := now.Before(firstWindowEnds)
	if inFirstWindow {
		windowLimit = l.FirstWindow
	}

	resets := now
	if window.Oldest != nil {
		resets = window.Oldest.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
	}
	// A first-window workspace resets at the earlier of the two boundaries:
	// its own first window ending, or the rolling window's natural reset.
	if inFirstWindow && firstWindowEnds.Before(resets) {
		resets = firstWindowEnds
	}

	return QuotaState{
		RemainingToday:  remaining(l.Daily, day.Used),
		RemainingWindow: remaining(windowLimit, window.Used),
		WindowResetsAt:  resets,
		Limits: QuotaReport{
			Daily:      l.Daily,
			Window:     windowLimit,
			WindowDays: l.WindowDays,
		},
	}, nil
}

func remaining(limit int, used int64) int {
	left := limit - int(used)
	if left < 0 {
		return 0
	}
	return left
}

func EnforceQuota(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID,
) (string, error) {
	return enforce(ctx, reader, l, workspaceID, allowance{
		sentinel: ErrQuotaExceeded, noun: "runs", prefix: "quota",
	})
}

type allowance struct {
	sentinel error
	noun     string
	prefix   string
}

func enforce(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID, a allowance,
) (string, error) {
	if !l.Enforced() {
		return "", nil
	}
	state, err := Usage(ctx, reader, l, workspaceID, time.Now())
	if err != nil {

		return a.prefix + "_unavailable", fmt.Errorf(
			"%w: an uncounted allowance is not treated as an unused one: %w",
			ErrAllowanceUnavailable, err)
	}
	if state.RemainingToday <= 0 {
		return a.prefix + "_daily",
			fmt.Errorf("%w: %d %s a day is the limit; it resets 24 hours after your earliest %s today",
				a.sentinel, l.Daily, a.noun, strings.TrimSuffix(a.noun, "s"))
	}
	if state.RemainingWindow <= 0 {
		return a.prefix + "_window",
			fmt.Errorf("%w: %d %s per %d days is the limit; the next one frees up at %s",
				a.sentinel, state.Limits.Window, a.noun, l.WindowDays,
				state.WindowResetsAt.UTC().Format(time.RFC3339))
	}
	return "", nil
}
