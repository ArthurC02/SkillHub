package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// An account that cannot cover the gateway ceiling is turned away before any
// run row exists, and the refusal is audited with its own reason.
func TestARunIsRefusedWhenTheBalanceCannotCoverItsCeiling(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	a.startingCredits = 0 // the only test in this file that wants an empty account
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
}

// Settlement charges what the gateway billed ($0.0382 -> 50 credits), not the
// 650-credit reservation, and once: the supervisor re-runs cleanup until `cleaned`.
func TestARunIsChargedWhatItSpentAndOnlyOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-settle")
	ctx := context.Background()

	finished := f.start(t)
	if err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	// Attached after Drive: the only gateway calls left are spend read and revoke.
	svc.Gateway = spendingGateway(t, 0.0382)

	for pass := 1; pass <= 2; pass++ {
		if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
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
}

// An unreadable spend charges nothing (never the whole ceiling) but still
// leaves an estimated cost row, so the Run stays visible.
func TestARunWhoseSpendIsUnreadableIsRecordedButNotCharged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-run-unreported")
	ctx := context.Background()

	finished := f.start(t)
	if err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	svc.Gateway = spendingGateway(t, -1) // rows with no spend field

	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
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

// spendingGateway serves the spend log and key deletion cleanup calls;
// spendUSD < 0 serves rows with no spend field.
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
	return &run.Gateway{AdminBaseURL: srv.URL, HTTP: srv.Client()}
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
