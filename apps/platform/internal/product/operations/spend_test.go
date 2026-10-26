package operations

import (
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAnUnpricedRunCountsAtLeastTheBudgetItsKeyWasGiven(t *testing.T) {
	budget := int64(5_000)
	for _, tc := range []struct {
		name string
		run  gen.PlatformAgentSpendSinceRow
		want int64
	}{
		{"every step priced", gen.PlatformAgentSpendSinceRow{PricedMicros: 1_200, KeyBudgetMicros: &budget}, 1_200},
		{"an unpriced step counts the whole key budget", gen.PlatformAgentSpendSinceRow{PricedMicros: 1_200, HasUnpriced: true, KeyBudgetMicros: &budget}, 5_000},
		{"priced spend above the budget still counts in full", gen.PlatformAgentSpendSinceRow{PricedMicros: 6_000, HasUnpriced: true, KeyBudgetMicros: &budget}, 6_000},
		{"a run without a recorded budget counts what was priced", gen.PlatformAgentSpendSinceRow{PricedMicros: 1_200, HasUnpriced: true}, 1_200},
	} {
		if got := countedSpend(tc.run); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestEnabledAgentCapsMayReachButNotPassTheirShareOfThePlatformBudget(t *testing.T) {
	override := int64(300_000)
	agents := []Agent{
		{Enabled: true, Definition: Definition{DailySpendCapMicros: 200_000}},
		{Enabled: true, Definition: Definition{DailySpendCapMicros: 200_000}, SpendCapOverrideMicros: &override},
		{Enabled: false, Definition: Definition{DailySpendCapMicros: 5_000_000}},
	}
	for _, c := range []struct {
		name      string
		allowance int64
		refused   bool
	}{
		{"the enabled total exactly at the allowance", 500_000, false},
		{"one micro-dollar over the allowance", 499_999, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := enabledCapsWithin(agents, c.allowance)

			over, refused := errors.AsType[*AgentCapsOverShare](err)
			if refused != c.refused {
				t.Fatalf("err = %v, want refused = %v", err, c.refused)
			}
			if refused && (over.EnabledMicros != 500_000 || over.AllowedMicros != c.allowance) {
				t.Fatalf("named %d over %d; want the enabled total 500000 (override counted, disabled not) over %d",
					over.EnabledMicros, over.AllowedMicros, c.allowance)
			}
		})
	}
}
