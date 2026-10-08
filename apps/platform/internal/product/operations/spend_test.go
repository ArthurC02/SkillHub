package operations

import (
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
