package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

const runBudgetPath = "/admin/settings/runs/model-budget"

func resetRunBudget(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), `DELETE FROM run_model_budget_setting`)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM run_model_budget_setting`) })
}

func modelBudgetOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	budget, ok := body["model_budget"].(map[string]any)
	if !ok {
		t.Fatalf("no model_budget in %v", body)
	}
	return budget
}

func requireTheDeploymentCeilingInItsBand(t *testing.T, operator *client) {
	t.Helper()
	code, body := operatorCall(t, operator, http.MethodGet, "/admin/settings/runs", "")
	b := modelBudgetOf(t, body)
	if code != http.StatusOK || b["credits"] != float64(650) || b["default_credits"] != float64(650) || b["reason"] != nil {
		t.Fatalf("default: %d %v, want the deployment's 650 credits that nobody set", code, body)
	}
	if b["min_credits"] != float64(65) || b["max_credits"] != float64(6500) {
		t.Fatalf("band = %v..%v, want 65..6500 credits (0.05 to 5 USD)", b["min_credits"], b["max_credits"])
	}
}

func TestAnOperatorSetsTheRunSpendCeilingWithinItsBoundsAndClearsIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunBudget(t, pool)
	operator := a.login(t, freshName("operator-run-budget"))
	a.auth.Operators = map[string]bool{operator.userID: true}

	requireTheDeploymentCeilingInItsBand(t, operator)

	refusesEach(t, operator, runBudgetPath, map[string]string{
		"just under the minimum": `{"credits":64,"note":"n"}`,
		"just over the maximum":  `{"credits":6501,"note":"n"}`,
		"no credits":             `{"note":"n"}`,
		"a blank note":           `{"credits":650,"note":"  "}`,
		"credits as words":       `{"credits":"many","note":"n"}`,
		"a fraction of a credit": `{"credits":65.5,"note":"n"}`,
	})

	for _, credits := range []float64{65, 6500} {
		raw, _ := json.Marshal(map[string]any{"credits": credits, "note": "bounds"})
		code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, string(raw))
		if b := modelBudgetOf(t, body); code != http.StatusOK || b["credits"] != credits || b["reason"] != "bounds" {
			t.Errorf("credits %v: %d %v", credits, code, body)
		}
	}

	if code, answer := operatorCall(t, operator, http.MethodDelete, runBudgetPath, `{"note":" "}`); code != http.StatusBadRequest {
		t.Errorf("clearing with a blank note: %d %v, want 400", code, answer)
	}
	code, body := operatorCall(t, operator, http.MethodDelete, runBudgetPath, `{"note":"back to the deployment value"}`)
	if b := modelBudgetOf(t, body); code != http.StatusOK || b["credits"] != float64(650) || b["reason"] != nil {
		t.Errorf("cleared: %d %v, want the deployment's 650 credits again", code, body)
	}

	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events WHERE action = $1 AND actor_user_id = $2 AND metadata->>'setting' = 'model_budget_usd'`,
		audit.ActionRunSettingsSet, mustUUID(t, operator.userID)).Scan(&n); err != nil || n != 3 {
		t.Errorf("audit events = %d (%v), want one per accepted change: two sets and one clear", n, err)
	}
}

func TestARunReservesAndKeepsTheCeilingInForceWhenItWasCreated(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunBudget(t, pool)
	a.startingCredits = 100
	fake, svc := haltHarness(t, a, pool)
	operator := a.login(t, freshName("operator-run-budget-snapshot"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	f := newFixture(t, a, pool, "alice-run-budget")
	startRun := func() (int, runView) {
		hash := f.confirmPermissions(t)
		return f.postJSON(t, "/skills/"+f.skillID+"/runs",
			`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+`","confirmed_summary_hash":"`+hash+`"}`)
	}

	if _, view := f.preflight(t); view.Blocked != "credit_balance" {
		t.Errorf("preflight under the default 0.50 ceiling is blocked by %q, want credit_balance", view.Blocked)
	}
	if code, body := startRun(); code != http.StatusUnprocessableEntity {
		t.Fatalf("100 credits against the default 0.50 ceiling: got %d (%+v), want 422", code, body)
	}
	if code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, `{"credits":65,"note":"small runs"}`); code != http.StatusOK {
		t.Fatalf("lower the ceiling: %d %v", code, body)
	}
	if _, view := f.preflight(t); view.Blocked == "credit_balance" {
		t.Error("preflight still says the balance cannot cover a run after the operator lowered the ceiling to 0.05")
	}
	code, created := startRun()
	if code != http.StatusCreated {
		t.Fatalf("100 credits against a 0.05 ceiling: got %d (%+v), want 201", code, created)
	}
	if code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, `{"credits":2600,"note":"bigger runs"}`); code != http.StatusOK {
		t.Fatalf("raise the ceiling: %d %v", code, body)
	}

	var kept float64
	if err := pool.QueryRow(context.Background(),
		`SELECT (policy_snapshot->>'model_budget_usd')::float8 FROM run_snapshots WHERE run_id = $1`,
		mustUUID(t, created.RunID)).Scan(&kept); err != nil || kept != 0.05 {
		t.Errorf("the run's ceiling = %v (%v), want the 0.05 it was created under", kept, err)
	}

	if err := driveThroughPolls(context.Background(), svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	sent, ok := fake.Request(created.RunID, 1)
	if !ok || sent.ModelGateway == nil || sent.ModelGateway.MaxBudgetUSD != 0.05 {
		t.Errorf("the first attempt's gateway grant = %+v (sent %v), want a 0.05 ceiling, not the 2.00 set after the run was created", sent.ModelGateway, ok)
	}
}

const runRateLimitPath = "/admin/settings/runs/tokens-per-minute"

func resetRunRateLimit(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), `DELETE FROM run_rate_limit_setting`)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM run_rate_limit_setting`) })
}

func tokensPerMinuteOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	rate, ok := body["tokens_per_minute"].(map[string]any)
	if !ok {
		t.Fatalf("no tokens_per_minute in %v", body)
	}
	return rate
}

func requireTheDeploymentRateLimitInItsBand(t *testing.T, operator *client) {
	t.Helper()
	code, body := operatorCall(t, operator, http.MethodGet, "/admin/settings/runs", "")
	if rate := tokensPerMinuteOf(t, body); code != http.StatusOK || rate["limit"] != float64(200000) ||
		rate["default_limit"] != float64(200000) || rate["min"] != float64(10000) || rate["max"] != float64(2000000) || rate["reason"] != nil {
		t.Fatalf("default: %d %v, want the deployment's 200000 within 10000..2000000 that nobody set", code, body)
	}
}

func TestAnOperatorSetsTheRunKeyTokensPerMinuteWithinItsBoundsAndClearsIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunRateLimit(t, pool)
	operator := a.login(t, freshName("operator-run-tpm"))
	a.auth.Operators = map[string]bool{operator.userID: true}

	requireTheDeploymentRateLimitInItsBand(t, operator)

	refusesEach(t, operator, runRateLimitPath, map[string]string{
		"just under the minimum": `{"limit":9999,"note":"n"}`,
		"just over the maximum":  `{"limit":2000001,"note":"n"}`,
		"no limit":               `{"note":"n"}`,
		"a blank note":           `{"limit":50000,"note":"  "}`,
	})

	for _, limit := range []float64{10000, 2000000} {
		raw, _ := json.Marshal(map[string]any{"limit": limit, "note": "bounds"})
		code, body := operatorCall(t, operator, http.MethodPut, runRateLimitPath, string(raw))
		if rate := tokensPerMinuteOf(t, body); code != http.StatusOK || rate["limit"] != limit || rate["reason"] != "bounds" {
			t.Errorf("limit %v: %d %v", limit, code, body)
		}
	}

	code, body := operatorCall(t, operator, http.MethodDelete, runRateLimitPath, `{"note":"back to the deployment value"}`)
	if rate := tokensPerMinuteOf(t, body); code != http.StatusOK || rate["limit"] != float64(200000) || rate["reason"] != nil {
		t.Errorf("cleared: %d %v, want the deployment's 200000 again", code, body)
	}

	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events WHERE action = $1 AND actor_user_id = $2 AND metadata->>'setting' = 'tokens_per_minute'`,
		audit.ActionRunSettingsSet, mustUUID(t, operator.userID)).Scan(&n); err != nil || n != 3 {
		t.Errorf("audit events = %d (%v), want one per accepted change: two sets and one clear", n, err)
	}
}

func TestARunAttemptKeyGetsTheTokensPerMinuteInForceWhenItIsIssued(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunRateLimit(t, pool)
	fake, svc := haltHarness(t, a, pool)
	operator := a.login(t, freshName("operator-run-tpm-key"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	f := newFixture(t, a, pool, "alice-run-tpm")

	created := f.start(t)
	if code, body := operatorCall(t, operator, http.MethodPut, runRateLimitPath, `{"limit":20000,"note":"provider throttling"}`); code != http.StatusOK {
		t.Fatalf("set the limit: %d %v", code, body)
	}
	if err := driveThroughPolls(context.Background(), svc.Drive, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	sent, ok := fake.Request(created.RunID, 1)
	if !ok || sent.ModelGateway == nil || sent.ModelGateway.TPMLimit != 20000 {
		t.Errorf("the attempt's gateway grant = %+v (sent %v), want the 20000 tokens per minute set before it was issued", sent.ModelGateway, ok)
	}
}
