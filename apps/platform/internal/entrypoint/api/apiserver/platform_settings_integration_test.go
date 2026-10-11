package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func spendCapAudit(t *testing.T, pool *pgxpool.Pool, name string) (from, to float64, count int) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT (metadata->>'from')::float8, (metadata->>'to')::float8 FROM audit_events
		WHERE action = $1 AND metadata->>'agent' = $2 ORDER BY id DESC`, audit.ActionAgentSpendCapSet, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var f, tt float64
		if err := rows.Scan(&f, &tt); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			from, to = f, tt
		}
		count++
	}
	return from, to, count
}

func wantLatestSpendCapAudit(t *testing.T, pool *pgxpool.Pool, name string, wantCount int, wantFrom, wantTo float64) {
	t.Helper()
	if from, to, n := spendCapAudit(t, pool, name); n != wantCount || from != wantFrom || to != wantTo {
		t.Errorf("latest audit from %v to %v (%d events), want event %d from %v to %v", from, to, n, wantCount, wantFrom, wantTo)
	}
}

func TestAnOperatorSetsAnAgentsDailySpendCapWithinTheCeilingAndClearsItBackToTheDefault(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	const name = "agent-spend-cap"
	registerTestAgent(t, pool, name)
	operator := a.login(t, freshName("operator-spend-cap"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	path := "/admin/agents/" + name + "/spend-cap"

	refusesEach(t, operator, path, map[string]string{
		"zero":                 `{"daily_spend_cap_usd_micros":0,"note":"n"}`,
		"one past the ceiling": `{"daily_spend_cap_usd_micros":5000001,"note":"n"}`,
		"a fraction":           `{"daily_spend_cap_usd_micros":1.5,"note":"n"}`,
		"no cap at all":        `{"note":"n"}`,
		"a blank note":         `{"daily_spend_cap_usd_micros":2000,"note":"  "}`,
	})
	if _, _, n := spendCapAudit(t, pool, name); n != 0 {
		t.Fatalf("refused changes wrote %d audit events", n)
	}
	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/agents/no-such-agent/spend-cap",
		`{"daily_spend_cap_usd_micros":2000,"note":"n"}`); code != http.StatusNotFound {
		t.Errorf("an unknown agent answered %d, want 404", code)
	}

	code, agent := operatorCall(t, operator, http.MethodPut, path, `{"daily_spend_cap_usd_micros":5000000,"note":"busy week"}`)
	if code != http.StatusOK || agent["daily_spend_cap_usd_micros"] != float64(5_000_000) ||
		agent["default_daily_spend_cap_usd_micros"] != float64(1) || agent["daily_spend_cap_overridden"] != true {
		t.Fatalf("setting the ceiling: %d %v, want the cap at 5000000 over a default of 1", code, agent)
	}
	wantLatestSpendCapAudit(t, pool, name, 1, 1, 5_000_000)

	if capMicros := capAfterReRegistering(t, pool, name); capMicros != 5_000_000 {
		t.Errorf("after the worker registered its agents again the cap is %d, want the operator's 5000000", capMicros)
	}

	code, agent = operatorCall(t, operator, http.MethodPut, path, `{"daily_spend_cap_usd_micros":null,"note":"back to normal"}`)
	if code != http.StatusOK || agent["daily_spend_cap_usd_micros"] != float64(1) || agent["daily_spend_cap_overridden"] != false {
		t.Errorf("clearing: %d %v, want the defined cap of 1 and no override", code, agent)
	}
	wantLatestSpendCapAudit(t, pool, name, 2, 5_000_000, 1)
}

func refusesEach(t *testing.T, operator *client, path string, bodies map[string]string) {
	t.Helper()
	for name, body := range bodies {
		if code, answer := operatorCall(t, operator, http.MethodPut, path, body); code != http.StatusBadRequest {
			t.Errorf("%s: got %d %v, want 400", name, code, answer)
		}
	}
}

func capAfterReRegistering(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	svc := &operations.Service{Pool: pool}
	if err := svc.Register(context.Background(), []operations.Definition{{
		Name: name, Purpose: "test agent", ModelRole: "skillhub-test", DailySpendCapMicros: 1,
		Tools: []string{"maintenance_report"},
	}}); err != nil {
		t.Fatal(err)
	}
	capMicros, err := svc.SpendCapMicros(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return capMicros
}

func clearEvaluationSettings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	clear := func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM evaluation_settings"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	clear()
	t.Cleanup(clear)
}

func TestAnOperatorSwitchesTheJudgePanelAndTheNextEvaluationReadsIt(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-judge-panel"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	svc := &eval.Service{Pool: pool}

	code, settings := operatorCall(t, operator, http.MethodGet, "/admin/settings", "")
	panel, _ := settings["judge_panel"].(map[string]any)
	if code != http.StatusOK || panel["enabled"] != false || panel["reason"] != nil {
		t.Fatalf("untouched settings: %d %v, want the panel off and no reason", code, settings)
	}

	refusesEach(t, operator, "/admin/settings/judge-panel", map[string]string{
		"no enabled":   `{"note":"n"}`,
		"a blank note": `{"enabled":true,"note":" "}`,
		"not JSON":     `on`,
	})
	if on, err := svc.JudgePanelEnabled(context.Background()); err != nil || on {
		t.Fatalf("a refused change switched the panel to %v (%v)", on, err)
	}

	code, settings = operatorCall(t, operator, http.MethodPut, "/admin/settings/judge-panel", `{"enabled":true,"note":"measure agreement"}`)
	panel, _ = settings["judge_panel"].(map[string]any)
	if code != http.StatusOK || panel["enabled"] != true || panel["reason"] != "measure agreement" || panel["set_by_user_id"] != operator.userID {
		t.Fatalf("switching on: %d %v", code, settings)
	}
	if on, err := svc.JudgePanelEnabled(context.Background()); err != nil || !on {
		t.Errorf("the next evaluation reads the panel as %v (%v), want on", on, err)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM audit_events WHERE action = $1 AND metadata->>'reason' = 'measure agreement' AND (metadata->>'to')::boolean",
		audit.ActionEvaluationSettingsSet); n != 1 {
		t.Errorf("switching on wrote %d matching audit events, want 1", n)
	}

	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/settings/judge-panel", `{"enabled":false,"note":"done"}`); code != http.StatusOK {
		t.Fatalf("switching off: %d", code)
	}
	if on, _ := svc.JudgePanelEnabled(context.Background()); on {
		t.Error("the panel is still on after the operator switched it off")
	}
}
