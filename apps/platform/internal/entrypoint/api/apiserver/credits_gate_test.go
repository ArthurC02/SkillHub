package apiserver

// Gate ① in isolation ("開始前：餘額 < 門檻就不讓開新會話", 負責人本輪定案):
// creditBalanceResponse is pure, so this needs no database.
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

// system.md:100 「擋住人的訊息必須說下一步是什麼」 — a block is never a bare
// refusal: the reason must name both the threshold and exactly how many
// credits are missing.
func TestCreditBalanceResponseBlocksBelowThresholdAndNamesTheDeficit(t *testing.T) {
	est := CreditSessionEstimate{LowCredits: 30, HighCredits: 65, ThresholdCredits: 65, SampleSize: 40}
	view := creditBalanceResponse(10, est)
	if view.CanStart {
		t.Fatalf("balance=10 threshold=65: CanStart = true, want false")
	}
	if view.BlockReason == "" {
		t.Fatal("blocked response carries no BlockReason at all")
	}
	for _, want := range []string{"10", "65", "55"} { // balance, threshold, deficit
		if !strings.Contains(view.BlockReason, want) {
			t.Errorf("BlockReason = %q; missing %q (balance/threshold/deficit must all be nameable)", view.BlockReason, want)
		}
	}
}

// A negative balance (allowed down to DebtFloorCredits by gate ②, which
// settleCost enforces elsewhere) must still produce a correct deficit and
// never be reported as if it were zero (負責人本輪：「絕不因讀不到成本而扣
// 0」's display-side twin — a negative balance is not nothing).
func TestCreditBalanceResponseHandlesANegativeBalance(t *testing.T) {
	est := CreditSessionEstimate{ThresholdCredits: 65, SampleSize: 25}
	view := creditBalanceResponse(-12, est)
	if view.BalanceCredits != -12 {
		t.Fatalf("BalanceCredits = %d, want -12 (a negative balance is not rounded up to zero)", view.BalanceCredits)
	}
	if view.CanStart {
		t.Fatal("a negative balance must never be allowed to start a session")
	}
	if !strings.Contains(view.BlockReason, "77") { // 65 - (-12) = 77
		t.Errorf("BlockReason = %q; want the deficit 77", view.BlockReason)
	}
	if view.DebtFloorCredits != DebtFloorCredits {
		t.Fatalf("DebtFloorCredits view = %d, want the package constant %d", view.DebtFloorCredits, DebtFloorCredits)
	}
}

// The fallback constant must be visibly marked as an estimate, not served
// silently as if it were a measured p95 (負責人本輪：「樣本數不足...在畫面
// 標明是估計值」).
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
