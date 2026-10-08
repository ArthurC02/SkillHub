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

	before := childPartitionNames(t, pool, analyticsTable)

	at, retention := date(2030, time.January, 15), 30*24*time.Hour
	plan, err := PlanMonthly(ctx, pool, analyticsTable, at, retention)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(plan.Dropped, "analytics_events_2026_09") || !contains(plan.Created, "analytics_events_2030_01") {
		t.Errorf("plan drops %v and creates %v, want September 2026 dropped and January 2030 created", plan.Dropped, plan.Created)
	}
	assertNames(t, "partitions after planning", childPartitionNames(t, pool, analyticsTable), before...)

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
