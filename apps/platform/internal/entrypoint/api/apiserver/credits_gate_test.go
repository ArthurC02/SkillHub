package apiserver

import (
	"strings"
	"testing"
)

func TestCreditBalanceResponseAllowsStartingASessionAtOrAboveTheThreshold(t *testing.T) {
	for _, balance := range []int64{65, 66, 1000} {
		est := CreditSessionEstimate{LowCredits: 30, HighCredits: 65, ThresholdCredits: 65, SampleSize: 40}
		view := creditBalanceResponse(balance, est)
		if !view.CanStart {
			t.Fatalf("balance=%d threshold=%d: CanStart = false, want true", balance, est.ThresholdCredits)
		}
		if view.BlockReason != "" {
			t.Fatalf("balance=%d: BlockReason = %q, want empty when a session may start", balance, view.BlockReason)
		}
	}
}

func TestCreditBalanceResponseBlocksBelowThresholdAndNamesTheDeficit(t *testing.T) {
	est := CreditSessionEstimate{LowCredits: 30, HighCredits: 65, ThresholdCredits: 65, SampleSize: 40}
	view := creditBalanceResponse(10, est)
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
	est := CreditSessionEstimate{ThresholdCredits: 65, SampleSize: 25}
	view := creditBalanceResponse(-12, est)
	if view.BalanceCredits != -12 {
		t.Fatalf("BalanceCredits = %d, want -12 (a negative balance is not rounded up to zero)", view.BalanceCredits)
	}
	if view.CanStart {
		t.Fatal("a negative balance must never be allowed to start a session")
	}
	if !strings.Contains(view.BlockReason, "77") {
		t.Errorf("BlockReason = %q; want the deficit 77", view.BlockReason)
	}
	if view.DebtFloorCredits != DebtFloorCredits {
		t.Fatalf("DebtFloorCredits view = %d, want the package constant %d", view.DebtFloorCredits, DebtFloorCredits)
	}
}

func TestCreditBalanceResponseCarriesTheEstimatedFlagThrough(t *testing.T) {
	fallback := CreditSessionEstimate{
		LowCredits: 30, HighCredits: FallbackSessionThresholdCredits,
		ThresholdCredits: FallbackSessionThresholdCredits, SampleSize: 3, Estimated: true,
	}
	view := creditBalanceResponse(100, fallback)
	if !view.EstimatedSession.Estimated {
		t.Fatal("a below-MinCostSampleSize estimate reached the view with Estimated = false")
	}
	if view.EstimatedSession.SampleSize != 3 {
		t.Fatalf("SampleSize = %d, want 3 (the real sample count, not hidden)", view.EstimatedSession.SampleSize)
	}

	measured := CreditSessionEstimate{ThresholdCredits: 50, SampleSize: MinCostSampleSize, Estimated: false}
	view = creditBalanceResponse(100, measured)
	if view.EstimatedSession.Estimated {
		t.Fatal("a measured (sample >= MinCostSampleSize) estimate was reported as Estimated = true")
	}
}
