package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func gaugeReading(t *testing.T, name string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return family.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("gauge %s is not registered", name)
	return 0
}

func TestTheModelBudgetObserverPublishesWhatTheGatewayReports(t *testing.T) {
	before := time.Now()
	w := &ModelBudgetObserveWorker{Read: func(context.Context) (run.PlatformBudget, error) {
		return run.PlatformBudget{SpentUSD: 41.5, LimitUSD: 50}, nil
	}}

	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if got := gaugeReading(t, "skillhub_model_platform_spend_usd"); got != 41.5 {
		t.Errorf("spend gauge = %v, want 41.5", got)
	}
	if got := gaugeReading(t, "skillhub_model_platform_budget_usd"); got != 50 {
		t.Errorf("budget gauge = %v, want 50", got)
	}
	if got := gaugeReading(t, "skillhub_model_platform_budget_read_timestamp_seconds"); got < float64(before.Unix()) {
		t.Errorf("read timestamp = %v, want at least %v", got, before.Unix())
	}
}

func TestAFailedBudgetReadLeavesTheLastReadingAndItsTimeAlone(t *testing.T) {
	ok := &ModelBudgetObserveWorker{Read: func(context.Context) (run.PlatformBudget, error) {
		return run.PlatformBudget{SpentUSD: 10, LimitUSD: 50}, nil
	}}
	if err := ok.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	readAt := gaugeReading(t, "skillhub_model_platform_budget_read_timestamp_seconds")
	unreachable := errors.New("gateway unreachable")
	failing := &ModelBudgetObserveWorker{Read: func(context.Context) (run.PlatformBudget, error) {
		return run.PlatformBudget{SpentUSD: 99, LimitUSD: 1}, unreachable
	}}

	if err := failing.Work(context.Background(), nil); !errors.Is(err, unreachable) {
		t.Fatalf("err = %v, want the read failure so the job is retried", err)
	}

	if got := gaugeReading(t, "skillhub_model_platform_spend_usd"); got != 10 {
		t.Errorf("spend gauge = %v after a failed read, want the last reading 10", got)
	}
	if got := gaugeReading(t, "skillhub_model_platform_budget_read_timestamp_seconds"); got != readAt {
		t.Errorf("read timestamp moved to %v after a failed read; staleness must stay visible", got)
	}
}
