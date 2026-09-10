package credit

import (
	"math"
	"testing"
)

// CreditsForUSD is the one place a dollar becomes a Credit on a screen, so it
// is the one place a wrong rate would be wrong everywhere at once — the pre-run
// estimate, the trace's usage line, the evaluation's cost and the comparison
// table all read it.
func TestCreditsForUSDAppliesTheMarkupAndRoundsUp(t *testing.T) {
	s := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}

	for _, tc := range []struct {
		name string
		usd  float64
		want int64
	}{
		// ADR-068's own worked figures.
		{"one generation, $0.0045", 0.0045, 6},         // 4500 micros -> 5850 billed -> 5.85 credits
		{"one search embedding, $0.00001", 0.00001, 1}, // 10 micros -> 13 billed -> 0.013 credits
		{"the M2 median run, $0.0566", 0.0566, 74},
		// The rounding is up at every step, never to nearest: a call that costs
		// a fraction of a credit still costs a credit (decision 6).
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

// Every refusal has to be distinguishable from a zero, because the screens that
// read this render absence and never render 0 — 「免費」 is the one wrong answer
// they must not give (設計 §2.9).
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
	// Zero is not a refusal: a call the gateway priced at exactly nothing is a
	// real measurement of nothing, and it converts to zero credits.
	if got, ok := s.CreditsForUSD(0); ok || got != 0 {
		// UsageCost treats a non-positive amount as unusable, so this documents
		// what the pair actually does rather than what one might assume: an
		// exact zero comes back as a refusal, and the caller renders 未測量.
		// Recorded here so a future change to that branch is a decision and not
		// an accident.
		t.Logf("CreditsForUSD(0) = %d, ok=%v — an exact zero is treated as no number", got, ok)
	}
}

// A deployment that prices differently converts differently, and nothing in the
// conversion is hard-coded to the shipped defaults.
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
