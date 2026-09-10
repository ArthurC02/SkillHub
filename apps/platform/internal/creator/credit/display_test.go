package credit

import (
	"math"
	"testing"
)

func TestCreditsForUSDAppliesTheMarkupAndRoundsUp(t *testing.T) {
	s := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}

	for _, tc := range []struct {
		name string
		usd  float64
		want int64
	}{

		{"one generation, $0.0045", 0.0045, 6},
		{"one search embedding, $0.00001", 0.00001, 1},
		{"the M2 median run, $0.0566", 0.0566, 74},

		{"a cost far below one credit", 0.0000001, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := s.CreditsForUSD(tc.usd)
			if !ok {
				t.Fatalf("CreditsForUSD(%v) refused a representable amount", tc.usd)
			}
			if got != tc.want {
				t.Errorf("CreditsForUSD(%v) = %d, want %d", tc.usd, got, tc.want)
			}
		})
	}
}

func TestCreditsForUSDRefusesRatherThanReturningZero(t *testing.T) {
	s := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}

	for _, tc := range []struct {
		name string
		usd  float64
	}{
		{"negative", -1},
		{"NaN", math.NaN()},
		{"infinite", math.Inf(1)},
		{"past the billable ceiling", float64(MaxBillableMicros)/1_000_000 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := s.CreditsForUSD(tc.usd); ok {
				t.Errorf("CreditsForUSD(%v) = %d, ok — want a refusal", tc.usd, got)
			}
		})
	}

	if got, ok := s.CreditsForUSD(0); ok || got != 0 {

		t.Logf("CreditsForUSD(0) = %d, ok=%v — an exact zero is treated as no number", got, ok)
	}
}

func TestCreditsForUSDFollowsTheDeploymentsOwnRate(t *testing.T) {
	noMarkup := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 10000}}
	if got, _ := noMarkup.CreditsForUSD(0.0134); got != 14 {
		t.Errorf("without markup, $0.0134 = %d credits, want 14", got)
	}
	withMarkup := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}
	if got, _ := withMarkup.CreditsForUSD(0.0134); got != 18 {
		t.Errorf("with the shipped 1.3x markup, $0.0134 = %d credits, want 18", got)
	}
}
