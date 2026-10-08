package partition

import (
	"context"
	"testing"
	"time"
)

func TestAPlanNamesExactlyThePartitionsTheRotationThenCreatesAndDrops(t *testing.T) {
	pool := requirePartitionDB(t)
	ctx := context.Background()
	const forever = 3650 * 24 * time.Hour
	if _, err := MaintainMonthly(ctx, pool, analyticsTable, date(2026, time.September, 1), forever); err != nil {
		t.Fatal(err)
	}

	at, retention := date(2026, time.November, 15), 30*24*time.Hour
	plan, err := PlanMonthly(ctx, pool, analyticsTable, at, retention)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "planned drops", plan.Dropped, "analytics_events_2026_08", "analytics_events_2026_09")
	assertNames(t, "planned creates", plan.Created, "analytics_events_2026_12", "analytics_events_2027_01")
	if !contains(childPartitionNames(t, pool, analyticsTable), "analytics_events_2026_08") {
		t.Fatal("planning dropped a partition")
	}

	done, err := MaintainMonthly(ctx, pool, analyticsTable, at, retention)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, "dropped", done.Dropped, plan.Dropped...)
	assertNames(t, "created", done.Created, plan.Created...)

	if _, err := PlanMonthly(ctx, pool, analyticsTable, at, 0); err == nil {
		t.Error("a plan with no retention window succeeded, want the same refusal as the rotation")
	}
}
