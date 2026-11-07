package apiserver_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/evalsettings"
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
	svc := &evalsettings.Service{Pool: pool}

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

func TestAnOperatorPricesAndSwitchesTheInjectionCheckWithoutRewritingThePanelsReason(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-injection-check"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	svc := &evalsettings.Service{Pool: pool}

	code, settings := operatorCall(t, operator, http.MethodGet, "/admin/settings", "")
	check, _ := settings["injection_check"].(map[string]any)
	if code != http.StatusOK || check["enabled"] != false || check["credits_per_million_tokens"] != nil || check["reason"] != nil {
		t.Fatalf("untouched settings: %d %v, want the check off, unpriced and unexplained", code, settings)
	}

	refusesEach(t, operator, "/admin/settings/injection-check", map[string]string{
		"no enabled":         `{"credits_per_million_tokens":50,"note":"n"}`,
		"no price":           `{"enabled":true,"note":"n"}`,
		"a price of zero":    `{"enabled":true,"credits_per_million_tokens":0,"note":"n"}`,
		"a price over a cap": `{"enabled":true,"credits_per_million_tokens":1000001,"note":"n"}`,
		"a blank note":       `{"enabled":true,"credits_per_million_tokens":50,"note":" "}`,
		"not JSON":           `on`,
	})
	if on, _, err := svc.InjectionCheckPolicy(context.Background()); err != nil || on {
		t.Fatalf("a refused change switched the check to %v (%v)", on, err)
	}

	settings = switchInjectionCheckOnAt(t, operator, "1", "1000000")
	panel, _ := settings["judge_panel"].(map[string]any)
	if panel["enabled"] != false || panel["reason"] != nil || panel["set_at"] != nil {
		t.Errorf("judge_panel = %v after only the injection check changed, want it off with no reason of its own", panel)
	}
	if on, credits, err := svc.InjectionCheckPolicy(context.Background()); err != nil || !on || credits != 1000000 {
		t.Errorf("the next evaluation reads the check as %v at %d credits (%v), want on at 1000000", on, credits, err)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events WHERE action = $1 AND metadata->>'setting' = 'injection_check'
		AND metadata->>'reason' = 'second signal' AND (metadata->>'to')::boolean`, audit.ActionEvaluationSettingsSet); n != 2 {
		t.Errorf("switching on twice wrote %d matching audit events, want 2", n)
	}

	expectSwitchingOffKeepsThePanel(t, operator)
}

func switchInjectionCheckOnAt(t *testing.T, operator *client, prices ...string) map[string]any {
	t.Helper()
	var settings map[string]any
	for _, price := range prices {
		var code int
		code, settings = operatorCall(t, operator, http.MethodPut, "/admin/settings/injection-check",
			`{"enabled":true,"credits_per_million_tokens":`+price+`,"note":"second signal"}`)
		check, _ := settings["injection_check"].(map[string]any)
		if code != http.StatusOK || check["enabled"] != true || check["credits_per_million_tokens"] != mustFloat(t, price) ||
			check["reason"] != "second signal" || check["set_at"] == nil {
			t.Fatalf("switching on at %s credits: %d %v", price, code, settings)
		}
	}
	return settings
}

func expectSwitchingOffKeepsThePanel(t *testing.T, operator *client) {
	t.Helper()
	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/settings/judge-panel", `{"enabled":true,"note":"panel"}`); code != http.StatusOK {
		t.Fatalf("switching the panel on: %d", code)
	}
	code, settings := operatorCall(t, operator, http.MethodPut, "/admin/settings/injection-check",
		`{"enabled":false,"credits_per_million_tokens":40,"note":"done"}`)
	check, _ := settings["injection_check"].(map[string]any)
	panel, _ := settings["judge_panel"].(map[string]any)
	if code != http.StatusOK || check["enabled"] != false || check["credits_per_million_tokens"] != float64(40) || check["reason"] != "done" {
		t.Fatalf("switching off: %d %v", code, settings)
	}
	if panel["enabled"] != true || panel["reason"] != "panel" {
		t.Errorf("judge_panel = %v after the check was switched off, want the panel's own change kept", panel)
	}
}

func TestAnOperatorSetsTheCandidatesPerRoundAndTheirCapWithinBounds(t *testing.T) {
	pool := requireDB(t)
	clearEvaluationSettings(t, pool)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("operator-reflection-candidates"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	svc := &evalsettings.Service{Pool: pool}
	path := "/admin/settings/reflection-candidates"

	code, settings := operatorCall(t, operator, http.MethodGet, "/admin/settings", "")
	rc, _ := settings["reflection_candidates"].(map[string]any)
	if _, capped := rc["round_credit_cap"]; code != http.StatusOK || rc["count"] != float64(1) || capped || rc["reason"] != nil {
		t.Fatalf("untouched settings: %d %v, want one candidate, no cap and no reason", code, settings)
	}

	refusesEach(t, operator, path, map[string]string{
		"no count":         `{"round_credit_cap":5,"note":"n"}`,
		"zero candidates":  `{"count":0,"note":"n"}`,
		"five candidates":  `{"count":5,"round_credit_cap":5,"note":"n"}`,
		"a cap of zero":    `{"count":2,"round_credit_cap":0,"note":"n"}`,
		"a cap over int32": `{"count":2,"round_credit_cap":2147483648,"note":"n"}`,
		"a blank note":     `{"count":2,"round_credit_cap":5,"note":" "}`,
		"not JSON":         `four`,
	})
	if policy, err := svc.ReflectionPolicy(context.Background()); err != nil || policy.Count != 1 || policy.RoundCreditCap != nil {
		t.Fatalf("a refused change left the policy at %+v (%v), want one candidate and no cap", policy, err)
	}

	for _, tc := range []struct {
		body  string
		count int
		cap   *int64
	}{
		{`{"count":4,"round_credit_cap":2147483647,"note":"widest"}`, 4, roundCap(2147483647)},
		{`{"count":1,"round_credit_cap":1,"note":"narrowest"}`, 1, roundCap(1)},
		{`{"count":2,"note":"cap cleared"}`, 2, nil},
	} {
		expectReflectionCandidatesSaved(t, operator, svc, tc.body, tc.count, tc.cap)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events WHERE action = $1 AND metadata->>'setting' = 'reflection_candidates' AND actor_user_id = $2`,
		audit.ActionEvaluationSettingsSet, mustUUID(t, operator.userID)); n != 3 {
		t.Errorf("three changes wrote %d audit events, want 3", n)
	}
}

func roundCap(n int64) *int64 { return &n }

func expectReflectionCandidatesSaved(t *testing.T, operator *client, svc *evalsettings.Service, body string, count int, roundCredits *int64) {
	t.Helper()
	var capShown any
	if roundCredits != nil {
		capShown = float64(*roundCredits)
	}
	code, settings := operatorCall(t, operator, http.MethodPut, "/admin/settings/reflection-candidates", body)
	rc, _ := settings["reflection_candidates"].(map[string]any)
	if code != http.StatusOK || rc["count"] != float64(count) || rc["round_credit_cap"] != capShown || rc["set_at"] == nil {
		t.Errorf("%s: %d %v", body, code, settings)
	}
	policy, err := svc.ReflectionPolicy(context.Background())
	if err != nil || policy.Count != count || !reflect.DeepEqual(policy.RoundCreditCap, roundCredits) {
		t.Errorf("%s: the next round reads %+v (%v)", body, policy, err)
	}
}
