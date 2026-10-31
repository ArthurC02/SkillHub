package apiserver_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func queuedBehind(n int) string {
	return fmt.Sprintf("前面還有 %d 個試跑在排隊；輪到時會自動開始，不必重新送出。", n)
}

func detailReason(t *testing.T, f fixture, runID string) string {
	t.Helper()
	detail, _, _ := reasonsShownFor(t, f, runID)
	return detail
}

func TestAQueuedRunSaysHowManyRunsWaitAheadOfIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	first := newFixture(t, a, pool, freshName("alice-queue-first"))
	second := newFixture(t, a, pool, freshName("bob-queue-second"))
	third := newFixture(t, a, pool, freshName("carol-queue-third"))
	firstRun, secondRun, thirdRun := first.start(t).RunID, second.start(t).RunID, third.start(t).RunID

	for _, tc := range []struct {
		name string
		f    fixture
		run  string
		want string
	}{
		{"first in line", first, firstRun, ""},
		{"one ahead", second, secondRun, queuedBehind(1)},
		{"two ahead", third, thirdRun, queuedBehind(2)},
	} {
		if got := detailReason(t, tc.f, tc.run); got != tc.want {
			t.Errorf("%s: detail says %q, want %q", tc.name, got, tc.want)
		}
	}

	mustExec(t, pool, `UPDATE runs SET cancel_requested_at = now() WHERE id = $1`, mustUUID(t, firstRun))
	if got := detailReason(t, third, thirdRun); got != queuedBehind(1) {
		t.Errorf("after the first run asked to cancel: detail says %q, want %q", got, queuedBehind(1))
	}

	mustExec(t, pool, `UPDATE runs SET status = 'provisioning' WHERE id = $1`, mustUUID(t, secondRun))
	if got := detailReason(t, third, thirdRun); got != "" {
		t.Errorf("after the second run left the queue: detail says %q, want nothing", got)
	}
}

func TestAQueuedRunsOwnReasonAndAPausedDispatchComeBeforeItsPlaceInLine(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	ahead := newFixture(t, a, pool, freshName("alice-queue-ahead"))
	behind := newFixture(t, a, pool, freshName("bob-queue-behind"))
	ahead.start(t)
	runID := behind.start(t).RunID

	mustExec(t, pool, `UPDATE runs SET status_reason = '等待中的原因' WHERE id = $1`, mustUUID(t, runID))
	if got := detailReason(t, behind, runID); got != "等待中的原因" {
		t.Errorf("with its own reason: detail says %q, want the run's own reason", got)
	}

	mustExec(t, pool, `UPDATE runs SET status_reason = NULL WHERE id = $1`, mustUUID(t, runID))
	pauseDispatch(t, pool)
	if got := detailReason(t, behind, runID); got != queuedWhilePaused {
		t.Errorf("while dispatch is paused: detail says %q, want %q", got, queuedWhilePaused)
	}
}

func TestTheRunQueueSummaryCountsOnlyRunsStillWaiting(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	f := newFixture(t, a, pool, freshName("queue-summary"))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var snapshotID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		SELECT workspace_id, id, user_prompt, acceptance_criteria, 'queue-summary'
		FROM test_cases WHERE id = $1 RETURNING id`, mustUUID(t, f.testCaseID)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	waitingSince := time.Now().Add(-5 * time.Minute).UTC().Truncate(time.Microsecond)
	for _, seeded := range []struct {
		status          string
		age             time.Duration
		cancelRequested bool
	}{
		{"queued", 5 * time.Minute, false},
		{"queued", time.Hour, true},
		{"running", 2 * time.Hour, false},
		{"failed", 3 * time.Hour, false},
	} {
		createdAt := waitingSince
		if seeded.age != 5*time.Minute {
			createdAt = time.Now().Add(-seeded.age)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, status, finished_at,
			                  created_at, cancel_requested_at)
			VALUES ($1, $2, $3, 'queue-summary', $4, $5, $6, CASE WHEN $7 THEN now() END)`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), snapshotID, seeded.status,
			finishedAtFor(seeded.status), createdAt, seeded.cancelRequested); err != nil {
			t.Fatal(err)
		}
	}

	got, err := gen.New(tx).SummarizeQueuedRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Queued != 1 {
		t.Errorf("queued = %d, want 1: a run asked to cancel, a dispatched run and a finished run do not wait", got.Queued)
	}
	if !got.OldestCreatedAt.Valid || !got.OldestCreatedAt.Time.Equal(waitingSince) {
		t.Errorf("oldest waiting since %v, want %v", got.OldestCreatedAt, waitingSince)
	}
}
