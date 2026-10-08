package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func TestDefaultPartitionTraceRowsOlderThanEveryKeptMonthAreDeletedAndTheKeptMonthStays(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	owner := a.login(t, "trace-retention-owner")
	runID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-retention-skill"))
	t.Cleanup(func() {
		tx, err := pool.Begin(context.Background())
		if err != nil {
			return
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		_, _ = tx.Exec(context.Background(), "SET LOCAL skillhub.purge = 'on'")
		_, _ = tx.Exec(context.Background(), `DELETE FROM trace_events WHERE run_id = $1`, runID)
		_ = tx.Commit(context.Background())
	})

	keptMonthStart := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	seedTraceEvent(t, pool, owner.workspaceID, runID, 1, keptMonthStart.Add(-time.Second), `[]`)
	seedTraceEvent(t, pool, owner.workspaceID, runID, 2, keptMonthStart, `[]`)

	cutoffMidJanuary := keptMonthStart.Add(14 * 24 * time.Hour)
	maintainedMidFebruary := cutoffMidJanuary.AddDate(0, 1, 0)
	report, err := trace.MaintainPartitions(ctx, pool, maintainedMidFebruary, maintainedMidFebruary.Sub(cutoffMidJanuary))
	t.Cleanup(func() {
		for _, created := range report.Created {
			_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+created)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	for seq, want := range map[int]int{1: 0, 2: 1} {
		var got int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM trace_events_default WHERE run_id = $1 AND seq = $2`, runID, seq).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("default-partition rows with seq %d after maintenance = %d, want %d", seq, got, want)
		}
	}
}
