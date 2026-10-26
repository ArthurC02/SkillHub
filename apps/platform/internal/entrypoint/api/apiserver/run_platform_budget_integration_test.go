package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func platformBudgetGateway(t *testing.T, answer func(http.ResponseWriter)) *run.Gateway {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user/info", func(w http.ResponseWriter, _ *http.Request) { answer(w) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return run.NewGateway(run.GatewayConfig{AdminBaseURL: srv.URL, AdminKey: "test", SandboxBaseURL: srv.URL, HTTP: srv.Client()})
}

func platformBudgetOf(body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestARunIsRefusedBeforeItStartsWhenThePlatformModelBudgetCannotCoverIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	svc.Gateway = platformBudgetGateway(t, platformBudgetOf(`{"user_info":{"spend":50.0,"max_budget":50.0}}`))
	f := newFixture(t, a, pool, "alice-platform-budget-spent")

	if _, view := f.preflight(t); view.Blocked != "platform_model_budget" {
		t.Errorf("preflight with the platform budget spent says blocked=%q, want platform_model_budget", view.Blocked)
	}
	hash := f.confirmPermissions(t)
	code, body := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST run with the platform budget spent: got %d (%+v), want 422", code, body)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM runs WHERE workspace_id = $1", mustUUID(t, f.workspaceID)); n != 0 {
		t.Errorf("a refused Run left %d run rows behind", n)
	}
	if fake.Live() != 0 {
		t.Errorf("a refused Run reached the sandbox (%d live)", fake.Live())
	}
	if got := refusalReasons(t, pool, f.workspaceID); len(got) != 1 || got[0] != "platform_model_budget" {
		t.Errorf("refusal reasons = %v, want exactly [platform_model_budget]", got)
	}
	if !strings.Contains(body.Error, "沒有扣點") || !strings.Contains(body.Error, "台北時間每天上午 8 點恢復") {
		t.Errorf("the refusal reads %q; it has to say nothing was charged and when the budget recovers", body.Error)
	}
}

func TestARunIsAdmittedWhenThePlatformModelBudgetHasRoom(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	svc.Gateway = platformBudgetGateway(t, platformBudgetOf(`{"user_info":{"spend":0.0,"max_budget":1000000.0}}`))
	f := newFixture(t, a, pool, "alice-platform-budget-room")

	if _, view := f.preflight(t); view.Blocked != "" {
		t.Errorf("preflight with room in the platform budget says blocked=%q, want nothing blocked", view.Blocked)
	}
}

func TestAnUnreadablePlatformModelBudgetDoesNotBlockARun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	svc.Gateway = platformBudgetGateway(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) })
	f := newFixture(t, a, pool, "alice-platform-budget-unreadable")

	if _, view := f.preflight(t); view.Blocked != "" {
		t.Errorf("preflight with an unreadable platform budget says blocked=%q; the gateway still enforces its budget, so nothing is blocked here", view.Blocked)
	}
}
