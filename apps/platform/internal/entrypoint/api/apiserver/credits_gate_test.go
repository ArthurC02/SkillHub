package apiserver

import (
	"strings"
	"testing"
)

const testSessionThreshold int64 = 65

const testMinSamples = 20

func TestCreditBalanceResponseShowsTheLedgersAnswerAndNoBlockReasonWhenAStartIsAllowed(t *testing.T) {
	est := CreditSessionEstimate{LowCredits: 30, HighCredits: 65, ThresholdCredits: 65, SampleSize: 40}
	view := creditBalanceResponse(65, true, est)
	if !view.CanStart {
		t.Fatal("the ledger allowed a start but the view says CanStart = false")
	}
	if view.BlockReason != "" {
		t.Fatalf("BlockReason = %q, want empty when a session may start", view.BlockReason)
	}
}

func TestCreditBalanceResponseBlocksBelowThresholdAndNamesTheDeficit(t *testing.T) {
	est := CreditSessionEstimate{LowCredits: 30, HighCredits: 65, ThresholdCredits: 65, SampleSize: 40}
	view := creditBalanceResponse(10, false, est)
	if view.CanStart {
		t.Fatalf("balance=10 threshold=65: CanStart = true, want false")
	}
	if view.BlockReason == "" {
		t.Fatal("blocked response carries no BlockReason at all")
	}
	for _, want := range []string{"10", "65", "55"} {
		if !strings.Contains(view.BlockReason, want) {
			t.Errorf("BlockReason = %q; missing %q (balance/threshold/deficit must all be nameable)", view.BlockReason, want)
		}
	}
}

func TestCreditBalanceResponseHandlesANegativeBalance(t *testing.T) {
	est := CreditSessionEstimate{ThresholdCredits: 65, SampleSize: 25, DebtFloorCredits: -80}
	view := creditBalanceResponse(-12, false, est)
	if view.BalanceCredits != -12 {
		t.Fatalf("BalanceCredits = %d, want -12 (a negative balance is not rounded up to zero)", view.BalanceCredits)
	}
	if view.DebtFloorCredits != -80 {
		t.Fatalf("DebtFloorCredits = %d, want the configured -80", view.DebtFloorCredits)
	}
	if view.CanStart {
		t.Fatal("a negative balance must never be allowed to start a session")
	}
	if !strings.Contains(view.BlockReason, "77") {
		t.Errorf("BlockReason = %q; want the deficit 77", view.BlockReason)
	}
}

func TestCreditBalanceResponseCarriesTheEstimatedFlagThrough(t *testing.T) {
	fallback := CreditSessionEstimate{
		LowCredits: 30, HighCredits: testSessionThreshold,
		ThresholdCredits: testSessionThreshold, SampleSize: 3, Estimated: true,
	}
	view := creditBalanceResponse(100, true, fallback)
	if !view.EstimatedSession.Estimated {
		t.Fatal("an estimate under 20 samples reached the view with Estimated = false")
	}
	if view.EstimatedSession.SampleSize != 3 {
		t.Fatalf("SampleSize = %d, want 3 (the real sample count, not hidden)", view.EstimatedSession.SampleSize)
	}

	measured := CreditSessionEstimate{ThresholdCredits: 50, SampleSize: testMinSamples, Estimated: false}
	view = creditBalanceResponse(100, true, measured)
	if view.EstimatedSession.Estimated {
		t.Fatal("a measured estimate (20 samples or more) was reported as Estimated = true")
	}
}
