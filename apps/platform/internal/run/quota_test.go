package run

import "testing"

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

// The proposed values, asserted so that changing one is a deliberate edit to a
// test as well as to a constant. All four are 待追認 — PDM-010 asks the owner to
// choose the first-window value explicitly rather than let an implementation
// infer it, and 20 is the proposal's own recommendation of the two.
func TestDefaultsAreThePDM010Proposal(t *testing.T) {
	l := DefaultQuotaLimits()
	if l.FirstWindow != 20 || l.Window != 30 || l.Daily != 5 || l.WindowDays != 30 {
		t.Errorf("defaults drifted from PDM-010 §8.1: %+v", l)
	}
	// The alternative reading PDM-010 names and refuses to pick for us.
	if l.FirstWindow == l.Window+20 {
		t.Error("the first window is the 20+30 reading; PDM-010 asks the owner to choose explicitly")
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
