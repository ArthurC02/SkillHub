package policy

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Enforcement and display move together (ADR-028 決策 3): the router mounts
// GET /me/quota and the pre-run summary carries a quota block only where this
// returns true. A build that shows an allowance it does not apply is the mistake
// 04 乙-2 records, and this predicate is the single place that cannot happen.
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

// The values, asserted so that changing one is a deliberate edit to a test as
// well as to a constant. All four were ratified 2026-08-27 exactly as proposed
// (m0/pdm-proposals.md §9.1, 05 R-1b), including the one PDM-010 refused to let
// an implementation infer: the first window is min(20,30) = 20, not 20+30 = 50.
// So the second assertion below is no longer guarding a gap in the proposal —
// it is guarding a ruling, which is the stronger reason to keep it.
func TestDefaultsAreThePDM010Proposal(t *testing.T) {
	l := DefaultQuotaLimits()
	if l.FirstWindow != 20 || l.Window != 30 || l.Daily != 5 || l.WindowDays != 30 {
		t.Errorf("defaults drifted from PDM-010 §8.1: %+v", l)
	}
	// The alternative reading, ruled against on 2026-08-27 — the comment moved,
	// the assertion and its message did not.
	if l.FirstWindow == l.Window+20 {
		t.Error("the first window is the 20+30 reading; the 2026-08-27 ruling took min(20,30) = 20")
	}
}

// remaining never goes below zero: a workspace that somehow spent more than its
// ceiling has none left, not a negative number on a screen.
func TestRemainingIsClamped(t *testing.T) {
	if got := remaining(5, 7); got != 0 {
		t.Errorf("remaining(5, 7) = %d, want 0", got)
	}
	if got := remaining(5, 2); got != 3 {
		t.Errorf("remaining(5, 2) = %d, want 3", got)
	}
}

// An unenforced allowance refuses nothing and touches no database — which is why
// EnforceQuota can be asked with a nil handle here. The paired half (a configured
// allowance actually refusing a run) needs real rows and lives in
// apiserver/beta_integration_test.go.
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
