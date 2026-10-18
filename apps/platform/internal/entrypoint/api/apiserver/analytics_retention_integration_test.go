package apiserver

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
)

func TestAnalyticsRowsPastRetentionAreDeletedWhileTheirMonthIsStillKept(t *testing.T) {
	pool := creditsTestPool(t)
	ctx := context.Background()
	now := time.Now().UTC()
	maintainedAt := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Add(20 * 24 * time.Hour)
	const retention = 10 * 24 * time.Hour
	if _, err := analytics.MaintainPartitions(ctx, pool, maintainedAt, 3650*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	forget := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM analytics_events WHERE session_id LIKE 'retention-%'`)
	}
	forget()
	t.Cleanup(forget)
	cutoff := maintainedAt.Add(-retention)
	insert := func(session string, at time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO analytics_events (event_name, session_id, occurred_at) VALUES ('session_started', $1, $2)`,
			session, at); err != nil {
			t.Fatal(err)
		}
	}
	insert("retention-past-cutoff", cutoff.Add(-time.Second))
	insert("retention-at-cutoff", cutoff)

	if _, err := analytics.MaintainPartitions(ctx, pool, maintainedAt, retention); err != nil {
		t.Fatal(err)
	}

	for session, want := range map[string]int{"retention-past-cutoff": 0, "retention-at-cutoff": 1} {
		var got int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM analytics_events WHERE session_id = $1`, session).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("rows for %s after maintenance = %d, want %d", session, got, want)
		}
	}
}

func TestPartitionMaintenanceWithoutARetentionWindowDeletesNoEvents(t *testing.T) {
	pool := creditsTestPool(t)
	ctx := context.Background()
	now := time.Now().UTC()
	maintainedAt := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Add(20 * 24 * time.Hour)
	if _, err := analytics.MaintainPartitions(ctx, pool, maintainedAt, 3650*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	forget := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM analytics_events WHERE session_id = 'retention-zero-window'`)
	}
	forget()
	t.Cleanup(forget)
	if _, err := pool.Exec(ctx,
		`INSERT INTO analytics_events (event_name, session_id, occurred_at) VALUES ('session_started', 'retention-zero-window', $1)`,
		maintainedAt.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := analytics.MaintainPartitions(ctx, pool, maintainedAt, 0); err == nil {
		t.Fatal("maintenance with no retention window was not refused")
	}

	var kept int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM analytics_events WHERE session_id = 'retention-zero-window'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("maintenance with no retention window left %d of 1 events", kept)
	}
}
