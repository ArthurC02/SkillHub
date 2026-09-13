package policy

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestQuotaEnforcedOnlyWithARealCeiling(t *testing.T) {
	cases := []struct {
		name string
		l    QuotaLimits
		want bool
	}{
		{"zero value ships enforcing nothing", QuotaLimits{}, false},
		{"no window length is no allowance", QuotaLimits{Daily: 5, Window: 30}, false},
		{"a window with no ceilings is no allowance", QuotaLimits{WindowDays: 30}, false},
		{"a daily ceiling alone is enough", QuotaLimits{Daily: 5, WindowDays: 30}, true},
		{"a window ceiling alone is enough", QuotaLimits{Window: 30, WindowDays: 30}, true},
		{"PDM-010 as proposed", DefaultQuotaLimits(), true},
	}
	for _, tc := range cases {
		if got := tc.l.Enforced(); got != tc.want {
			t.Errorf("%s: Enforced() = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestDefaultsAreThePDM010Proposal(t *testing.T) {
	l := DefaultQuotaLimits()
	if l.FirstWindow != 20 || l.Window != 30 || l.Daily != 5 || l.WindowDays != 30 {
		t.Errorf("defaults drifted from PDM-010 §8.1: %+v", l)
	}

	if l.FirstWindow == l.Window+20 {
		t.Error("the first window is the 20+30 reading; the 2026-08-27 ruling took min(20,30) = 20")
	}
}

func TestRemainingIsClamped(t *testing.T) {
	if got := remaining(5, 7); got != 0 {
		t.Errorf("remaining(5, 7) = %d, want 0", got)
	}
	if got := remaining(5, 2); got != 3 {
		t.Errorf("remaining(5, 2) = %d, want 3", got)
	}
}

func TestEnforceQuotaIsSilentWhenNotConfigured(t *testing.T) {
	reason, err := EnforceQuota(context.Background(), UsageReader{}, QuotaLimits{}, pgtype.UUID{})
	if reason != "" || err != nil {
		t.Errorf("EnforceQuota with no allowance = (%q, %v), want (\"\", nil)", reason, err)
	}
}

func TestUsageRefusesWithoutOwnerReaders(t *testing.T) {
	if _, err := Usage(context.Background(), UsageReader{}, DefaultQuotaLimits(), pgtype.UUID{}, time.Now()); err == nil {
		t.Error("Usage succeeded without run and identity owner readers")
	}
}

func TestUsageCombinesOwnerFactsWithoutPersistenceTypes(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-10 * 24 * time.Hour)
	counts := 0
	state, err := Usage(context.Background(), UsageReader{
		WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) {
			return now.Add(-40 * 24 * time.Hour), nil
		},
		CountRuns: func(context.Context, pgtype.UUID, time.Time) (RunUsage, error) {
			counts++
			if counts == 1 {
				return RunUsage{Used: 4, Oldest: &oldest}, nil
			}
			return RunUsage{Used: 2}, nil
		},
	}, DefaultQuotaLimits(), pgtype.UUID{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if counts != 2 || state.RemainingToday != 3 || state.RemainingWindow != 26 {
		t.Fatalf("unexpected quota state after owner reads: counts=%d state=%+v", counts, state)
	}
	wantReset := oldest.Add(30 * 24 * time.Hour)
	if !state.WindowResetsAt.Equal(wantReset) {
		t.Errorf("window resets at %s, want %s", state.WindowResetsAt, wantReset)
	}
}

func splitReader(window, today int64, created time.Time) UsageReader {
	dayStart := time.Now().Add(-25 * time.Hour)
	return UsageReader{
		WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) { return created, nil },
		CountRuns: func(_ context.Context, _ pgtype.UUID, since time.Time) (RunUsage, error) {
			if since.Before(dayStart) {
				return RunUsage{Used: window}, nil
			}
			return RunUsage{Used: today}, nil
		},
	}
}

func TestTheRunAllowanceRefusesWithExactlyTheReasonsItPublishes(t *testing.T) {
	ctx := context.Background()
	old := time.Now().Add(-365 * 24 * time.Hour)
	limits := DefaultQuotaLimits()

	published := []string{"quota_unavailable", "quota_daily", "quota_window"}
	if got := AllowanceRefusalReasons(RunQuotaPrefix); !slices.Equal(got, published) {
		t.Fatalf("AllowanceRefusalReasons = %v, want %v; these strings are a label on a "+
			"published counter and in the audit log, so renaming one is renaming a series", got, published)
	}
	emitted := map[string]bool{}
	for _, tc := range []struct {
		name   string
		reader UsageReader
	}{
		{"uncountable", UsageReader{
			WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) {
				return time.Time{}, errors.New("the counter is unreachable")
			},
			CountRuns: func(context.Context, pgtype.UUID, time.Time) (RunUsage, error) {
				return RunUsage{}, nil
			},
		}},
		{"daily spent", generateReader(int64(limits.Daily), old)},
		{"window spent", splitReader(int64(limits.Window), 0, old)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, err := EnforceQuota(ctx, tc.reader, limits, pgtype.UUID{})
			if err == nil {
				t.Fatalf("%s was allowed through", tc.name)
			}
			if !slices.Contains(published, reason) {
				t.Fatalf("reason = %q, which AllowanceRefusalReasons does not publish: %v", reason, published)
			}
			emitted[reason] = true
		})
	}
	for _, reason := range published {
		if !emitted[reason] {
			t.Errorf("%q is published as a refusal reason but no path produces it", reason)
		}
	}
}
