package creation

import "testing"

func TestTheLastApprovedStepCanBeSpentAndNoneAfterIt(t *testing.T) {
	limits := testLimits()
	for _, c := range []struct {
		name   string
		steps  int
		wantOK bool
	}{
		{"one step left", limits.MaxSteps - 1, true},
		{"every step taken", limits.MaxSteps, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := Snapshot{Steps: c.steps, BudgetUSD: limits.MaxCostUSD}
			if got := canSpend(p, limits); got != c.wantOK {
				t.Fatalf("%d of %d steps taken with budget left: canSpend = %v, want %v", c.steps, limits.MaxSteps, got, c.wantOK)
			}
		})
	}
}
