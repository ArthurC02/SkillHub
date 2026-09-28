package credit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingStatistics struct {
	*fakeStore
	reads int
	err   error
}

func (c *countingStatistics) RecentStatistics(ctx context.Context, kind CostKind) (Statistics, error) {
	c.reads++
	if c.err != nil {
		return Statistics{}, c.err
	}
	return c.fakeStore.RecentStatistics(ctx, kind)
}

func freshStatistics(samples int) Statistics {
	return Statistics{SampleCount: samples, P50UsdMicros: 10_000, P95UsdMicros: 40_000, WindowEnd: time.Now()}
}

func TestEstimateReadsTheStatisticsOnceAndPricesBothEnds(t *testing.T) {
	stale := freshStatistics(MinStatSamples)
	stale.WindowEnd = time.Now().Add(-MaxStatisticsAge - time.Minute)
	unbillable := freshStatistics(MinStatSamples)
	unbillable.P95UsdMicros = MaxBillableMicros + 1
	unbillableLow := freshStatistics(MinStatSamples)
	unbillableLow.P50UsdMicros = -1

	for _, tc := range []struct {
		name  string
		stats *Statistics
		want  Estimate
	}{
		{"nothing recorded yet", nil,
			Estimate{LowCredits: 70, HighCredits: 70, ThresholdCredits: 70, Estimated: true}},
		{"one sample short of the minimum", ptr(freshStatistics(MinStatSamples - 1)),
			Estimate{LowCredits: 70, HighCredits: 70, ThresholdCredits: 70, Estimated: true}},
		{"exactly the minimum", ptr(freshStatistics(MinStatSamples)),
			Estimate{LowCredits: 13, HighCredits: 52, ThresholdCredits: 52, SampleCount: MinStatSamples}},
		{"a window older than the limit", &stale,
			Estimate{LowCredits: 70, HighCredits: 70, ThresholdCredits: 70, Estimated: true}},
		{"a high end that cannot be billed", &unbillable,
			Estimate{LowCredits: 70, HighCredits: 70, ThresholdCredits: 70, Estimated: true}},
		{"a low end that cannot be billed keeps the high end", &unbillableLow,
			Estimate{LowCredits: 52, HighCredits: 52, ThresholdCredits: 52, SampleCount: MinStatSamples}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &countingStatistics{fakeStore: newFakeStore()}
			if tc.stats != nil {
				store.stats[KindCreationSession] = *tc.stats
			}
			svc := &Service{Store: store, Config: testConfig()}

			got, err := svc.Estimate(context.Background(), KindCreationSession)
			if err != nil {
				t.Fatalf("Estimate: %v", err)
			}
			if got != tc.want {
				t.Errorf("Estimate = %+v, want %+v", got, tc.want)
			}
			if store.reads != 1 {
				t.Errorf("the statistics were read %d times, want 1", store.reads)
			}
		})
	}
}

func TestEstimateReportsAStoreThatCannotAnswer(t *testing.T) {
	broken := errors.New("connection refused")
	svc := &Service{Store: &countingStatistics{fakeStore: newFakeStore(), err: broken}, Config: testConfig()}

	got, err := svc.Estimate(context.Background(), KindCreationSession)
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the store's own error", err)
	}
	if got != (Estimate{}) {
		t.Errorf("Estimate = %+v alongside an error, want the zero value", got)
	}
}

func TestEstimateWithoutAStoreIsUnavailable(t *testing.T) {
	_, err := (&Service{Config: testConfig()}).Estimate(context.Background(), KindCreationSession)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func ptr[T any](v T) *T { return &v }
