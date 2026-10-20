package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestARunIsRefusedWhenTheBalanceCannotCoverItsCeiling(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	a.startingCredits = 0
	fake, _ := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-no-credit")

	hash := f.confirmPermissions(t)
	code, body := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST run with no credit: got %d (%+v), want 422", code, body)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM runs WHERE workspace_id = $1", mustUUID(t, f.workspaceID)); n != 0 {
		t.Errorf("a refused Run left %d run rows behind", n)
	}
	if fake.Live() != 0 {
		t.Errorf("a refused Run reached the sandbox (%d live)", fake.Live())
	}
	if got := refusalReasons(t, pool, f.workspaceID); len(got) != 1 || got[0] != "credit_balance" {
		t.Errorf("refusal reasons = %v, want exactly [credit_balance]", got)
	}
	msg := body.Error
	if !strings.Contains(msg, "點數不足") || !strings.Contains(msg, "已經開始的試跑不受影響") {
		t.Errorf("the refusal reads %q; it has to say what ran out and what is unaffected", msg)
	}
}

func TestARunIsChargedWhatItSpentAndOnlyOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-settle")
	ctx := context.Background()

	finished := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}

	svc.Gateway = spendingGateway(t, 0.0382)

	for pass := 1; pass <= 2; pass++ {
		if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
			t.Fatalf("cleanup pass %d: %v", pass, err)
		}
	}

	runID := mustUUID(t, finished.RunID)
	var entries int
	var charged int64
	if err := pool.QueryRow(ctx, `
		SELECT count(*), coalesce(-sum(delta_credits), 0)
		FROM credit_entries WHERE kind = 'debit' AND ref_type = 'run' AND ref_id = $1`,
		runID).Scan(&entries, &charged); err != nil {
		t.Fatal(err)
	}
	if entries != 1 {
		t.Fatalf("debit entries for this run = %d after two cleanup passes, want exactly 1", entries)
	}
	if charged != 50 {
		t.Errorf("charged %d credits, want 50 ($0.0382 at 1.3x and US$0.001 per credit) — not the 650-credit reservation", charged)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM cost_events WHERE kind = 'run' AND ref_id = $1", runID); n != 1 {
		t.Errorf("cost events for this run = %d, want 1", n)
	}
	if got := balanceOf(t, pool, f.userID); got != betaGrantCredits-50 {
		t.Errorf("balance = %d, want %d", got, betaGrantCredits-50)
	}
	if !costSettled(t, pool, runID) {
		t.Error("a run charged at its real cost is not marked settled, so supervision would keep re-reading its spend")
	}
}

func TestARunWhoseSpendIsUnreadableIsRecordedButNotCharged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-unreported")
	ctx := context.Background()

	finished := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	svc.Gateway = spendingGateway(t, -1)

	if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	runID := mustUUID(t, finished.RunID)
	if n := countRow(t, pool, "SELECT count(*) FROM credit_entries WHERE ref_type = 'run' AND ref_id = $1", runID); n != 0 {
		t.Errorf("an unreadable spend produced %d debit entries, want none", n)
	}
	if n := countRow(t, pool,
		"SELECT count(*) FROM cost_events WHERE kind = 'run' AND ref_id = $1 AND cost_source = 'estimated' AND usd_micros = 0",
		runID); n != 1 {
		t.Errorf("estimated zero-cost events for this run = %d, want 1 — the Run must stay visible", n)
	}
	if got := balanceOf(t, pool, f.userID); got != betaGrantCredits {
		t.Errorf("balance = %d, want the untouched %d", got, betaGrantCredits)
	}
}

func TestARunWhoseSpendIsReadOnALaterCleanupReachesTheLedgerAtItsRealCost(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-read-later")
	ctx := context.Background()

	finished := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	for _, spend := range []float64{-1, 0.0382} {
		svc.Gateway = spendingGateway(t, spend)
		if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
			t.Fatalf("cleanup with spend %v: %v", spend, err)
		}
	}

	runID := mustUUID(t, finished.RunID)
	if n := countRow(t, pool,
		"SELECT count(*) FROM cost_events WHERE kind = 'run' AND ref_id = $1 AND cost_source = 'gateway' AND usd_micros > 0",
		runID); n != 1 {
		t.Errorf("gateway-priced cost events for this run = %d, want 1 — the spend read later must reach the ledger the statistics read", n)
	}
	if got := balanceOf(t, pool, f.userID); got != betaGrantCredits-50 {
		t.Errorf("balance = %d, want %d", got, betaGrantCredits-50)
	}
}

type recordingQueue struct {
	mu      sync.Mutex
	cleaned []run.RunWork
}

func (*recordingQueue) Drive(context.Context, run.RunWork) (bool, error)     { return false, nil }
func (*recordingQueue) DriveInTx(context.Context, pgx.Tx, run.RunWork) error { return nil }
func (*recordingQueue) CleanInTx(context.Context, pgx.Tx, run.RunWork) error { return nil }

func (q *recordingQueue) Clean(_ context.Context, w run.RunWork) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cleaned = append(q.cleaned, w)
	return nil
}

func (q *recordingQueue) took(runID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	found := slices.ContainsFunc(q.cleaned, func(w run.RunWork) bool { return w.RunID.String() == runID })
	q.cleaned = nil
	return found
}

func costSettled(t *testing.T, pool *pgxpool.Pool, runID any) bool {
	t.Helper()
	var settled bool
	if err := pool.QueryRow(context.Background(), "SELECT cost_settled_at IS NOT NULL FROM runs WHERE id = $1", runID).Scan(&settled); err != nil {
		t.Fatal(err)
	}
	return settled
}

func ageFinishedRun(t *testing.T, pool *pgxpool.Pool, runID any, interval string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, `SET session_replication_role = DEFAULT`); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := conn.Exec(ctx, "UPDATE runs SET finished_at = now() - $2::interval, spend_rechecked_at = NULL WHERE id = $1",
		runID, interval); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisionReReadsAnUnreadableRunSpendForADayUntilItIsCharged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-recheck")
	ctx := context.Background()
	finished := f.start(t)
	runID := mustUUID(t, finished.RunID)
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, f.workspaceID), runID); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	svc.Gateway = spendingGateway(t, -1)
	if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), runID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if costSettled(t, pool, runID) {
		t.Fatal("a run whose spend was unreadable is marked settled, so nothing would ever read it again")
	}

	queue := &recordingQueue{}
	rechecks := run.SpendRechecks{Queries: gen.New(pool), Queue: queue}
	finishedAgo := func(interval string) { ageFinishedRun(t, pool, runID, interval) }
	enqueued := func() bool {
		t.Helper()
		if err := rechecks.Enqueue(ctx); err != nil {
			t.Fatal(err)
		}
		return queue.took(finished.RunID)
	}

	finishedAgo("24 hours 1 minute")
	if enqueued() {
		t.Error("a run that finished more than a day ago was queued for another spend read")
	}
	finishedAgo("23 hours 59 minutes")
	svc.Queue = queue
	if err := svc.Supervise(ctx); err != nil {
		t.Logf("supervision reported other runs' trouble: %v", err)
	}
	if !queue.took(finished.RunID) {
		t.Fatal("supervision did not queue a run inside the day with an unread spend for another read")
	}
	if enqueued() {
		t.Error("the same run was queued again at once instead of after the recheck interval")
	}

	svc.Gateway = spendingGateway(t, 0.0382)
	if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), runID); err != nil {
		t.Fatalf("the queued cleanup: %v", err)
	}
	if got := balanceOf(t, pool, f.userID); got != betaGrantCredits-50 {
		t.Errorf("balance = %d, want %d once the spend became readable", got, betaGrantCredits-50)
	}
	finishedAgo("2 minutes")
	if enqueued() || !costSettled(t, pool, runID) {
		t.Error("a run charged at its real cost is still queued for spend reads")
	}
}

func spendingGateway(t *testing.T, spendUSD float64) *run.Gateway {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /spend/logs/v2", func(w http.ResponseWriter, _ *http.Request) {
		row := map[string]any{"prompt_tokens": 19_215, "completion_tokens": 300}
		if spendUSD >= 0 {
			row["spend"] = spendUSD
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{row}, "total_pages": 1})
	})
	mux.HandleFunc("POST /key/delete", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return run.NewGateway(run.GatewayConfig{AdminBaseURL: srv.URL, AdminKey: "test", SandboxBaseURL: srv.URL, HTTP: srv.Client()})
}

func balanceOf(t *testing.T, pool *pgxpool.Pool, userID string) int64 {
	t.Helper()
	var balance int64
	if err := pool.QueryRow(context.Background(),
		"SELECT coalesce((SELECT balance_credits FROM credit_accounts WHERE user_id = $1), 0)",
		mustUUID(t, userID)).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func TestCleanupRevokesTheAttemptKeyBeforeReadingWhatTheRunSpent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-revoke-first")
	ctx := context.Background()
	finished := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}

	var mu sync.Mutex
	var calls []string
	note := func(call string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /spend/logs/v2", func(w http.ResponseWriter, _ *http.Request) {
		note("read spend")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "spend": 0.01}}, "total_pages": 1,
		})
	})
	mux.HandleFunc("POST /key/delete", func(w http.ResponseWriter, _ *http.Request) {
		note("revoke")
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	svc.Gateway = run.NewGateway(run.GatewayConfig{AdminBaseURL: srv.URL, AdminKey: "test", SandboxBaseURL: srv.URL, HTTP: srv.Client()})

	if err := svc.CleanRun(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	revoked, read := slices.Index(calls, "revoke"), slices.Index(calls, "read spend")
	if revoked < 0 || read < 0 || revoked > read {
		t.Errorf("gateway calls = %v, want the key revoked before the spend is read; spend after the read would go uncharged", calls)
	}
}
