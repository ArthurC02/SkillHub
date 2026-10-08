package capacity

import (
	"math"
	"testing"
	"time"
)

func TestParseRestoreRate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		want     RestoreRate
		rejected bool
	}{
		{name: "unset falls back to the unmeasured default", raw: "", want: RestoreRate{BytesPerSecond: DefaultRestoreBytesPerSecond}},
		{name: "one byte per second is the smallest measured rate", raw: "1", want: RestoreRate{BytesPerSecond: 1, Measured: true}},
		{name: "surrounding spaces are ignored", raw: " 104857600 ", want: RestoreRate{BytesPerSecond: 104857600, Measured: true}},
		{name: "zero is rejected", raw: "0", rejected: true},
		{name: "a negative rate is rejected", raw: "-5", rejected: true},
		{name: "a unit suffix is rejected", raw: "100MB", rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRestoreRate(tc.raw)
			if tc.rejected {
				if err == nil {
					t.Fatalf("ParseRestoreRate(%q) = %+v, want an error", tc.raw, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ParseRestoreRate(%q) = %+v, %v; want %+v", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestTheRestoreBudgetIsTenMinutesAtTheRestoreRate(t *testing.T) {
	if got := (RestoreRate{BytesPerSecond: 3}).BudgetBytes(); got != 1800 {
		t.Fatalf("budget at 3 B/s = %d, want 1800", got)
	}
}

func samplesOf(bytes ...int64) []Sample {
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	samples := make([]Sample, len(bytes))
	for i, b := range bytes {
		samples[i] = Sample{Day: start.AddDate(0, 0, i), Bytes: b}
	}
	return samples
}

func TestDaysUntilBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		samples []Sample
		budget  int64
		want    float64
	}{
		{name: "no samples never reaches the budget", samples: nil, budget: 1000, want: math.Inf(1)},
		{name: "a single sample has no trend", samples: samplesOf(500), budget: 1000, want: math.Inf(1)},
		{name: "a flat database never reaches the budget", samples: samplesOf(500, 500, 500), budget: 1000, want: math.Inf(1)},
		{name: "a shrinking database never reaches the budget", samples: samplesOf(700, 600, 500), budget: 1000, want: math.Inf(1)},
		{name: "steady growth reaches it in the remaining bytes over bytes per day", samples: samplesOf(100, 200, 300), budget: 1000, want: 7},
		{name: "a database exactly at the budget has no days left", samples: samplesOf(800, 900, 1000), budget: 1000, want: 0},
		{name: "a flat database exactly at the budget has no days left", samples: samplesOf(1000, 1000), budget: 1000, want: 0},
		{name: "a database one byte under the budget still has time", samples: samplesOf(799, 899, 999), budget: 1000, want: 0.01},
		{name: "a noisy sample bends the trend instead of setting it", samples: samplesOf(100, 300, 200, 400), budget: 1200, want: 10},
		{name: "a shrinking database already over the budget has no days left", samples: samplesOf(1300, 1200), budget: 1000, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DaysUntilBudget(tc.samples, tc.budget); math.Abs(got-tc.want) > 1e-9 && !(math.IsInf(got, 1) && math.IsInf(tc.want, 1)) {
				t.Fatalf("DaysUntilBudget = %v, want %v", got, tc.want)
			}
		})
	}
}
