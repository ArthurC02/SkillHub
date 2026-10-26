package run

import "testing"

func TestThePlatformBudgetCoversARunOnlyAfterTheCeilingsOfUnfinishedRuns(t *testing.T) {
	cases := []struct {
		name       string
		budget     PlatformBudget
		unfinished int64
		want       bool
	}{
		{"exactly one ceiling left and nothing in flight", PlatformBudget{LimitUSD: 50, SpentUSD: 49.75}, 0, true},
		{"a cent short of one ceiling", PlatformBudget{LimitUSD: 50, SpentUSD: 49.76}, 0, false},
		{"room for two ceilings with one run in flight", PlatformBudget{LimitUSD: 50, SpentUSD: 49.5}, 1, true},
		{"room for two ceilings with two runs in flight", PlatformBudget{LimitUSD: 50, SpentUSD: 49.5}, 2, false},
		{"spent past the limit", PlatformBudget{LimitUSD: 50, SpentUSD: 51}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.budget.covers(c.unfinished, 0.25); got != c.want {
				t.Fatalf("covers(%d unfinished, 0.25) with %+v = %v, want %v", c.unfinished, c.budget, got, c.want)
			}
		})
	}
}
