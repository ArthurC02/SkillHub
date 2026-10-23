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

func TestAnOperatorSetsTheRunSpendCeilingWithinItsBoundsAndClearsIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunBudget(t, pool)
	operator := a.login(t, freshName("operator-run-budget"))
	a.auth.Operators = map[string]bool{operator.userID: true}

	code, body := operatorCall(t, operator, http.MethodGet, "/admin/settings/runs", "")
	if b := modelBudgetOf(t, body); code != http.StatusOK || b["usd"] != b["default_usd"] || b["usd"] != 0.5 || b["reason"] != nil {
		t.Fatalf("default: %d %v, want the deployment's 0.50 that nobody set", code, body)
	}

	refusesEach(t, operator, runBudgetPath, map[string]string{
		"just under the minimum": `{"usd":0.049,"note":"n"}`,
		"just over the maximum":  `{"usd":5.01,"note":"n"}`,
		"no usd":                 `{"note":"n"}`,
		"a blank note":           `{"usd":1,"note":"  "}`,
		"usd as words":           `{"usd":"one","note":"n"}`,
	})

	for _, usd := range []float64{0.05, 5} {
		raw, _ := json.Marshal(map[string]any{"usd": usd, "note": "bounds"})
		code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, string(raw))
		if b := modelBudgetOf(t, body); code != http.StatusOK || b["usd"] != usd || b["reason"] != "bounds" {
			t.Errorf("usd %v: %d %v", usd, code, body)
		}
	}

	if code, answer := operatorCall(t, operator, http.MethodDelete, runBudgetPath, `{"note":" "}`); code != http.StatusBadRequest {
		t.Errorf("clearing with a blank note: %d %v, want 400", code, answer)
	}
	code, body = operatorCall(t, operator, http.MethodDelete, runBudgetPath, `{"note":"back to the deployment value"}`)
	if b := modelBudgetOf(t, body); code != http.StatusOK || b["usd"] != 0.5 || b["reason"] != nil {
		t.Errorf("cleared: %d %v, want the deployment's 0.50 again", code, body)
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
	if code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, `{"usd":0.05,"note":"small runs"}`); code != http.StatusOK {
		t.Fatalf("lower the ceiling: %d %v", code, body)
	}
	if _, view := f.preflight(t); view.Blocked == "credit_balance" {
		t.Error("preflight still says the balance cannot cover a run after the operator lowered the ceiling to 0.05")
	}
	code, created := startRun()
	if code != http.StatusCreated {
		t.Fatalf("100 credits against a 0.05 ceiling: got %d (%+v), want 201", code, created)
	}
	if code, body := operatorCall(t, operator, http.MethodPut, runBudgetPath, `{"usd":2,"note":"bigger runs"}`); code != http.StatusOK {
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
