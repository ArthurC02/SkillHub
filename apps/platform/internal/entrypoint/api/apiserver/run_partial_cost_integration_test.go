package apiserver_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestPartlyPricedRunWaitsForTheRemainingCostAndChargesOnlyOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	svc.Deployment.SettlesWithoutSpendWait = true
	f := newFixture(t, a, pool, "alice-partial-run-cost")
	ctx := t.Context()
	finished := f.start(t)
	runID := mustUUID(t, finished.RunID)
	workspaceID := mustUUID(t, f.workspaceID)
	if err := driveThroughPolls(ctx, svc.Drive, workspaceID, runID); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []struct {
		name   string
		rows   string
		pages  int
		priced bool
	}{
		{"known plus unknown", `[{"spend":0.03},{"spend":null}]`, 1, false},
		{"overflowed price sum", `[{"spend":1e308},{"spend":1e308}]`, 1, false},
		{"truncated log", `[{"spend":0.03},{"spend":0.0082}]`, 6, false},
		{"invalid negative price", `[{"spend":0.03},{"spend":-0.0082}]`, 1, false},
		{"both known", `[{"spend":0.03},{"spend":0.0082}]`, 1, true},
		{"cleanup replay", `[{"spend":0.03},{"spend":0.0082}]`, 1, true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/spend/logs/v2" {
				_, _ = fmt.Fprintf(w, `{"data":%s,"total_pages":%d}`, stage.rows, stage.pages)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}))
		svc.Gateway = run.NewGateway(run.GatewayConfig{AdminBaseURL: server.URL, AdminKey: "test", SandboxBaseURL: server.URL, HTTP: server.Client()})
		err := svc.CleanRun(ctx, workspaceID, runID)
		server.Close()
		if err != nil {
			t.Fatalf("%s cleanup: %v", stage.name, err)
		}
		if got := costSettled(t, pool, runID); got != stage.priced {
			t.Fatalf("%s: partial cost was treated as final: settled=%v", stage.name, got)
		}
		var entries int
		var charged int64
		if err := pool.QueryRow(ctx, `SELECT count(*), coalesce(-sum(delta_credits),0) FROM credit_entries WHERE kind = 'debit' AND ref_type = 'run' AND ref_id = $1`, runID).Scan(&entries, &charged); err != nil {
			t.Fatal(err)
		}
		wantEntries, wantCharge := 0, int64(0)
		if stage.priced {
			wantEntries, wantCharge = 1, 50
		}
		if entries != wantEntries || charged != wantCharge || balanceOf(t, pool, f.userID) != betaGrantCredits-wantCharge {
			t.Fatalf("%s: incorrect ledger charge: entries=%d charged=%d", stage.name, entries, charged)
		}
		query := "SELECT count(*) FROM cost_events WHERE kind = 'run' AND ref_id = $1 AND cost_source = 'estimated' AND usd_micros = 0"
		if stage.priced {
			query = "SELECT count(*) FROM cost_events WHERE kind = 'run' AND ref_id = $1 AND cost_source = 'gateway' AND usd_micros = 38200"
		}
		if got := countRow(t, pool, query, runID); got != 1 {
			t.Fatalf("%s: cost evidence lost its source or value: events=%d", stage.name, got)
		}
	}
}
