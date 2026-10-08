package jobruns

import (
	"testing"
	"time"
)

func TestOverdueRatio(t *testing.T) {
	since := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, tc := range []struct {
		name   string
		now    time.Time
		period time.Duration
		want   float64
	}{
		{name: "just succeeded", now: since, period: day, want: 0},
		{name: "half a period later", now: since.Add(12 * time.Hour), period: day, want: 0.5},
		{name: "exactly one period later", now: since.Add(day), period: day, want: 1},
		{name: "two missed periods", now: since.Add(3 * day), period: day, want: 3},
		{name: "a clock behind the record is not negative", now: since.Add(-time.Hour), period: day, want: 0},
		{name: "a job without a period is never overdue", now: since.Add(day), period: 0, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OverdueRatio(since, tc.period, tc.now); got != tc.want {
				t.Fatalf("OverdueRatio = %v, want %v", got, tc.want)
			}
		})
	}
}
