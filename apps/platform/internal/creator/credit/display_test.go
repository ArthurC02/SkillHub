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
		{"past int64 once scaled to micros", 1e13},
		{"largest float", math.MaxFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := s.CreditsForUSD(tc.usd); ok {
				t.Errorf("CreditsForUSD(%v) = %d, ok — want a refusal", tc.usd, got)
			}
		})
	}

	if got, ok := s.CreditsForUSD(0); ok || got != 0 {
		t.Errorf("CreditsForUSD(0) = %d, ok=%v; want a refusal, an exact zero is treated as no number", got, ok)
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

func TestCreditsWithinUSDRefusesAnAmountPastTheBillableRangeWithoutWrapping(t *testing.T) {
	s := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}
	for _, tc := range []struct {
		name        string
		usd         float64
		wantCredits int64
		wantOK      bool
	}{
		{"the largest billable amount is accepted", 1000, 1_300_000, true},
		{"just past the largest billable amount is refused", 1000.00001, 0, false},
		{"an amount past what int64 micros can hold is refused", 1e13, 0, false},
		{"the largest float is refused", math.MaxFloat64, 0, false},
		{"zero is accepted and worth nothing", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := s.CreditsWithinUSD(tc.usd)
			if got != tc.wantCredits || ok != tc.wantOK {
				t.Fatalf("CreditsWithinUSD(%v) = (%d, %v), want (%d, %v)", tc.usd, got, ok, tc.wantCredits, tc.wantOK)
			}
		})
	}
}

func TestABudgetEnteredInCreditsComesBackAsTheSameCredits(t *testing.T) {
	s := &Service{Config: Config{MicrosPerCredit: 1000, MarkupBps: 13000}}
	for credits := int64(1); credits <= 20000; credits++ {
		usd, ok := s.USDForCredits(credits)
		if !ok {
			t.Fatalf("USDForCredits(%d) refused", credits)
		}
		if back, _ := s.CreditsForUSD(usd); back != credits {
			t.Fatalf("%d credits -> $%v -> %d credits", credits, usd, back)
		}
	}
	if got, _ := s.CreditsWithinUSD(1); got != 1300 {
		t.Errorf("CreditsWithinUSD($1) = %d, want 1300", got)
	}
	if usd, _ := s.USDForCredits(1300); usd > 1 {
		t.Errorf("the ceiling shown as credits is worth $%v, more than $1", usd)
	}
}
