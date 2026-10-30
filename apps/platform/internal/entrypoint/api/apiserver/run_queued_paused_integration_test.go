package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const queuedWhilePaused = "平台暫時停止開始新的試跑；恢復後這次試跑會自動開始，不必重新送出。"

func pauseDispatch(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	var haltID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO dispatch_halts (provider, source, reason) VALUES ('', 'orphan_threshold', 'held for the queued-reason test')
		RETURNING id::text`).Scan(&haltID); err != nil {
		t.Fatal(err)
	}
	lift := func() {
		mustExec(t, pool, `UPDATE dispatch_halts SET lifted_at = now(), lift_reason = 'test over' WHERE id = $1 AND lifted_at IS NULL`,
			mustUUID(t, haltID))
	}
	t.Cleanup(lift)
	return lift
}

func reasonsShownFor(t *testing.T, f fixture, runID string) (detail, listed, traced string) {
	t.Helper()
	code, run := f.getRun(t, runID)
	if code != http.StatusOK {
		t.Fatalf("GET /runs/%s: %d", runID, code)
	}
	code, list := f.doJSON(t, http.MethodGet, "/runs?test_case_id="+f.testCaseID, "")
	if code != http.StatusOK {
		t.Fatalf("GET /runs: %d", code)
	}
	listedReason := "missing from the list"
	for _, item := range list["runs"].([]any) {
		row := item.(map[string]any)
		if row["run_id"] == runID {
			listedReason, _ = row["status_reason"].(string)
		}
	}
	code, trace := f.doJSON(t, http.MethodGet, "/runs/"+runID+"/trace", "")
	if code != http.StatusOK {
		t.Fatalf("GET /runs/%s/trace: %d", runID, code)
	}
	tracedReason, _ := trace["status_reason"].(string)
	return run.StatusReason, listedReason, tracedReason
}

func TestAQueuedRunSaysDispatchIsPausedOnlyWhileItIs(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	f := newFixture(t, a, pool, freshName("alice-queued-paused"))
	runID := f.start(t).RunID

	for _, phase := range []struct {
		name string
		act  func()
		want string
	}{
		{"dispatching", func() {}, ""},
		{"paused", func() { pauseDispatch(t, pool) }, queuedWhilePaused},
	} {
		phase.act()
		detail, listed, traced := reasonsShownFor(t, f, runID)
		for surface, got := range map[string]string{"detail": detail, "list": listed, "trace": traced} {
			if got != phase.want {
				t.Errorf("%s: %s says %q, want %q", phase.name, surface, got, phase.want)
			}
		}
	}
}

func TestDispatchPauseLeavesAnyOtherRunReasonAlone(t *testing.T) {
	for _, tc := range []struct {
		name, status, reason string
	}{
		{"queued run with its own reason", "queued", "等待中的原因"},
		{"provisioning run", "provisioning", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := requireDB(t)
			a := newAPI(t, pool)
			clearRunBacklog(t, pool)
			f := newFixture(t, a, pool, freshName("alice-paused-other"))
			runID := f.start(t).RunID
			mustExec(t, pool, `UPDATE runs SET status = $2, status_reason = NULLIF($3, '') WHERE id = $1`,
				mustUUID(t, runID), tc.status, tc.reason)
			pauseDispatch(t, pool)

			detail, listed, traced := reasonsShownFor(t, f, runID)
			for surface, got := range map[string]string{"detail": detail, "list": listed, "trace": traced} {
				if got != tc.reason {
					t.Errorf("%s says %q, want %q", surface, got, tc.reason)
				}
			}
		})
	}
}
