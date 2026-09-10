package partition

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestUpcomingMonthsCoverThisMonthAndTwoMore(t *testing.T) {
	got := upcomingMonths(date(2026, time.November, 21))
	want := []time.Time{
		date(2026, time.November, 1),
		date(2026, time.December, 1),
		date(2027, time.January, 1),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("upcomingMonths = %v, want %v", got, want)
	}
}

func TestExpiredMonthsUsesTheMonthsEndNotItsStart(t *testing.T) {
	existing := []string{
		"analytics_events_2026_08",
		"analytics_events_2026_09",
		"analytics_events_2026_10",
		"analytics_events_default",
	}

	got := expiredMonths("analytics_events", existing, date(2026, time.November, 15), 30*24*time.Hour)
	want := []string{"analytics_events_2026_08", "analytics_events_2026_09"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expiredMonths = %v, want %v", got, want)
	}
}

func TestExpiredMonthsNeverNamesTheDefaultOrAForeignPartition(t *testing.T) {
	existing := []string{
		"analytics_events_default",
		"analytics_events_archive",
		"analytics_events_2026_13",
		"analytics_events_2026_012",
	}

	if got := expiredMonths("analytics_events", existing, date(2126, time.January, 1), time.Hour); len(got) != 0 {
		t.Fatalf("expiredMonths would drop %v", got)
	}
}

func TestGuardsRefuseBeforeAnyStatementRuns(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"trace_events; DROP TABLE skills", "public.trace_events", "Trace_Events", ""} {
		if _, err := MaintainMonthly(ctx, nil, bad, date(2026, time.August, 21), time.Hour); err == nil {
			t.Errorf("table name %q accepted", bad)
		}
	}
	for _, bad := range []time.Duration{0, -time.Hour} {
		if _, err := MaintainMonthly(ctx, nil, "trace_events", date(2026, time.August, 21), bad); err == nil {
			t.Errorf("retention %s accepted", bad)
		}
	}
}
