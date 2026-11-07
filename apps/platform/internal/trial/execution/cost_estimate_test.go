package run

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type statisticsLedger struct {
	CreditLedger
	stats RunCostStatistics
	err   error
}

func (statisticsLedger) CreditsForUSD(usd float64) (int64, bool) {
	return int64(math.Round(usd * 1000)), true
}

func (l statisticsLedger) RunCostStatistics(context.Context) (RunCostStatistics, error) {
	return l.stats, l.err
}

func TestThePreflightEstimateUsesTheRunStatisticsTheAdmissionUsesAndTheBaselineOtherwise(t *testing.T) {
	baseline := [3]int64{10, 60, 300}
	for _, tc := range []struct {
		name   string
		ledger statisticsLedger
		want   [3]int64
		basis  string
	}{
		{"enough recent runs", statisticsLedger{stats: RunCostStatistics{MedianCredits: 42, ThresholdCredits: 180, SampleCount: 37, Reliable: true}},
			[3]int64{42, 42, 180}, "最近 37 次試跑"},
		{"too few recent runs", statisticsLedger{stats: RunCostStatistics{MedianCredits: 42, ThresholdCredits: 180, SampleCount: 3}},
			baseline, "M2 基準試跑"},
		{"the statistics cannot be read", statisticsLedger{err: errors.New("cost statistics unavailable")},
			baseline, "M2 基準試跑"},
	} {
		got, err := runCostEstimate(context.Background(), tc.ledger)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if figures := [3]int64{got.LowCredits, got.TypicalCredits, got.HighCredits}; figures != tc.want {
			t.Errorf("%s: low, typical, high = %v, want %v", tc.name, figures, tc.want)
		}
		if !strings.Contains(got.Basis, tc.basis) {
			t.Errorf("%s: basis %q does not name %q", tc.name, got.Basis, tc.basis)
		}
	}
}
