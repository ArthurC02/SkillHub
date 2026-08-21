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

// The months a write can land in between two runs of a monthly cron. If this
// only reached the next month, one missed run would put every event in the
// default partition, which is the state db/migrations/0019 describes and which
// no partition drop can undo.
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

// A partition is expired only when every row it can hold is older than the
// window, so the comparison is against the month's exclusive upper bound. Using
// the start instead would drop a month whose last days are still in window.
func TestExpiredMonthsUsesTheMonthsEndNotItsStart(t *testing.T) {
	existing := []string{
		"analytics_events_2026_08",
		"analytics_events_2026_09",
		"analytics_events_2026_10",
		"analytics_events_default",
	}
	// now - 30d = 2026-10-16. August ends 09-01 and September ends 10-01, both
	// at or before the cutoff. October ends 11-01 and is still in window even
	// though its first days are not.
	got := expiredMonths("analytics_events", existing, date(2026, time.November, 15), 30*24*time.Hour)
	want := []string{"analytics_events_2026_08", "analytics_events_2026_09"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expiredMonths = %v, want %v", got, want)
	}
}

// The default partition is the one partition that must never be dropped: it is
// where everything lands when a month is missing, so dropping it turns a
// recoverable backlog into lost events. It is safe here because it cannot parse
// as a month — this test pins that consequence rather than the mechanism, since
// any future naming change has to keep it true.
func TestExpiredMonthsNeverNamesTheDefaultOrAForeignPartition(t *testing.T) {
	existing := []string{
		"analytics_events_default",  // must survive any retention window
		"analytics_events_archive",  // attached by hand, not ours to drop
		"analytics_events_2026_13",  // not a month
		"analytics_events_2026_012", // not the shape monthName produces
	}
	// A century of retention would still expire a real month from 2026.
	if got := expiredMonths("analytics_events", existing, date(2126, time.January, 1), time.Hour); len(got) != 0 {
		t.Fatalf("expiredMonths would drop %v", got)
	}
}

// Both guards run before the pool is touched, which is what lets this test pass
// a nil pool. A table name that is not a bare identifier is a bug, not input:
// every caller is a constant in the owning context.
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
