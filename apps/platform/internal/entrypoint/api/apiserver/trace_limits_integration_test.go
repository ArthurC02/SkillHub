package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func TestTheAdvancedTracePagesAtAThousandEventsAndPointsAtTheRest(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-page-size")
	runID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-page-size-skill"))
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO trace_events
		(event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type, source,
		 schema_version, masked, masked_fields, payload, late)
		SELECT gen_random_uuid(), $1, $2, 1, n,
		       now() - (n * interval '1 second'), 'script_log', 'sandbox',
		       '1.0', true, '[]', '{}', false
		FROM generate_series(1, 1001) AS n`, owner.workspaceID, runID); err != nil {
		t.Fatal(err)
	}

	code, first := owner.advancedTrace(t, runID)
	if code != http.StatusOK || len(first.Events) != 1000 || !first.HasMore || first.NextAfter <= 0 {
		t.Fatalf("first page: %d with %d events, has_more=%v, next_after=%d; want 1000 events, more to come and a cursor",
			code, len(first.Events), first.HasMore, first.NextAfter)
	}
	code, rest := owner.advancedTraceAfter(t, runID, first.NextAfter)
	if code != http.StatusOK || len(rest.Events) != 1 || rest.HasMore {
		t.Fatalf("second page: %d with %d events, has_more=%v; want the one remaining event and no more",
			code, len(rest.Events), rest.HasMore)
	}
}

func TestTracePartitionMaintenanceWithoutARetentionWindowDeletesNoEvents(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	owner := a.login(t, "trace-zero-retention")
	runID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-zero-retention-skill"))
	seedTraceEvent(t, pool, owner.workspaceID, runID, 1, time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC), `[]`)

	if _, err := trace.MaintainPartitions(ctx, pool, time.Now(), 0); err == nil {
		t.Fatal("maintenance with a zero retention window succeeded, want it refused")
	}

	var kept int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trace_events WHERE run_id = $1`, runID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("events left after a refused zero-window maintenance = %d, want 1", kept)
	}
}
