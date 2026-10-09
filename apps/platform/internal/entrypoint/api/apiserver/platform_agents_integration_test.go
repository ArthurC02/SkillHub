package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

func registerTestAgent(t *testing.T, pool *pgxpool.Pool, name string) *operations.Service {
	t.Helper()
	ctx := context.Background()
	svc := &operations.Service{Pool: pool}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM platform_agent_brake"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		for _, stmt := range []string{
			"DELETE FROM platform_agent_runs WHERE agent_id IN (SELECT id FROM platform_agents WHERE name = $1)",
			"DELETE FROM platform_agents WHERE name = $1",
		} {
			if _, err := pool.Exec(ctx, stmt, name); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})
	if err := svc.Register(ctx, []operations.Definition{{
		Name: name, Purpose: "test agent", ModelRole: "skillhub-test", DailySpendCapMicros: 1,
		Tools: []string{"maintenance_report"},
	}}); err != nil {
		t.Fatal(err)
	}
	return svc
}

func runStatus(t *testing.T, pool *pgxpool.Pool, run pgtype.UUID) (status, reason string) {
	t.Helper()
	var why *string
	if err := pool.QueryRow(context.Background(),
		"SELECT status, reason FROM platform_agent_runs WHERE id = $1", run).Scan(&status, &why); err != nil {
		t.Fatal(err)
	}
	if why != nil {
		reason = *why
	}
	return status, reason
}

func switchAgent(t *testing.T, operator *client, name string, enabled bool, note string) map[string]any {
	t.Helper()
	body := `{"enabled":false,"note":"` + note + `"}`
	if enabled {
		body = `{"enabled":true,"note":"` + note + `"}`
	}
	code, agent := operatorCall(t, operator, http.MethodPut, "/admin/agents/"+name+"/enabled", body)
	if code != http.StatusOK {
		t.Fatalf("PUT enabled=%v: got %d %v, want 200", enabled, code, agent)
	}
	return agent
}

func agentAuditCount(t *testing.T, pool *pgxpool.Pool, action string) int {
	t.Helper()
	return countRow(t, pool, "SELECT count(*) FROM audit_events WHERE action = $1", action)
}

func TestTheAuditLogTellsAnAgentFromAPersonAndRefusesBoth(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	const name = "agent-audit-actor"
	registerTestAgent(t, pool, name)
	operator := a.login(t, "operator-agent-audit")
	a.auth.Operators = map[string]bool{operator.userID: true}
	agent := switchAgent(t, operator, name, true, "audit actor")

	var agentID, operatorID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM platform_agents WHERE name = $1", name).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	if err := operatorID.Scan(operator.userID); err != nil {
		t.Fatal(err)
	}
	resource := pgtype.UUID{Bytes: [16]byte{0xa9, 0x7e}, Valid: true}
	if err := audit.Log(ctx, pool, audit.Event{
		Agent: agentID, Action: audit.ActionDispatchHalt, ResourceType: audit.ResourceDispatch, ResourceID: resource,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), "SET LOCAL skillhub.purge = 'on'"); err != nil {
				return err
			}
			_, err := tx.Exec(context.Background(), "DELETE FROM audit_events WHERE actor_agent_id = $1", agentID)
			return err
		}); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if err := audit.Log(ctx, pool, audit.Event{
		Actor: operatorID, Agent: agentID, Action: audit.ActionDispatchHalt, ResourceType: audit.ResourceDispatch,
	}); err == nil {
		t.Error("an audit event naming both a person and an agent was stored")
	}

	byResource := map[string]map[string]any{}
	for _, event := range allAuditEvents(t, operator) {
		if id, ok := event["resource_id"].(string); ok {
			byResource[id] = event
		}
	}
	byAgent := byResource[pgconv.UUIDString(resource)]
	if byAgent["actor_kind"] != "agent" || byAgent["actor_agent_id"] != pgconv.UUIDString(agentID) || byAgent["actor_user_id"] != nil {
		t.Errorf("the agent's event reads as %v, want actor_kind agent naming the agent and no person", byAgent)
	}
	byPerson := byResource[pgconv.UUIDString(agentID)]
	if byPerson["actor_kind"] != "person" || byPerson["actor_user_id"] != operator.userID || byPerson["actor_agent_id"] != nil {
		t.Errorf("the operator's enable reads as %v (agent %v), want actor_kind person naming the operator", byPerson, agent)
	}
}

func TestPlatformAgentRoutesAreInvisibleWithoutTheOperatorRole(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	registerTestAgent(t, pool, "agent-route-404")

	member := a.login(t, "member-agents-404")
	anon := &client{Client: http.DefaultClient, base: a.URL}
	for _, c := range []struct {
		name string
		cl   *client
	}{{"anonymous", anon}, {"member", member}} {
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/admin/agents", ""},
			{http.MethodPut, "/admin/agents/agent-route-404/enabled", `{"enabled":true,"note":"n"}`},
			{http.MethodPut, "/admin/agents/brake", `{"note":"n"}`},
			{http.MethodDelete, "/admin/agents/brake", `{"note":"n"}`},
		} {
			if code, _ := operatorCall(t, c.cl, tc.method, tc.path, tc.body); code != http.StatusNotFound {
				t.Errorf("%s %s as %s: got %d, want 404", tc.method, tc.path, c.name, code)
			}
		}
	}
	if n := countRow(t, pool, "SELECT count(*) FROM platform_agents WHERE name = 'agent-route-404' AND enabled"); n != 0 {
		t.Error("a refused call enabled the agent")
	}
	if n := countRow(t, pool, "SELECT count(*) FROM platform_agent_brake"); n != 0 {
		t.Error("a refused call engaged the brake")
	}
}

func TestDisablingAnAgentStopsItsRunBeforeTheNextStep(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	const name = "agent-disable-stops"
	svc := registerTestAgent(t, pool, name)
	operator := a.login(t, "operator-agent-disable")
	a.auth.Operators = map[string]bool{operator.userID: true}

	if _, err := svc.StartRun(ctx, name); !errors.Is(err, agentloop.ErrHalted) {
		t.Fatalf("a registered agent nobody enabled started a run: %v", err)
	}

	if enabled := switchAgent(t, operator, name, true, "first run"); enabled["enabled"] != true || enabled["owner_user_id"] != operator.userID {
		t.Fatalf("enable: got %v, want enabled and owned by the operator", enabled)
	}
	run, err := svc.StartRun(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BeforeStep(ctx, run); err != nil {
		t.Fatalf("an enabled agent's run was stopped: %v", err)
	}

	switchAgent(t, operator, name, false, "misbehaving")
	if err := svc.BeforeStep(ctx, run); !errors.Is(err, agentloop.ErrHalted) {
		t.Fatalf("the next step after a disable: got %v, want ErrAgentHalted", err)
	}
	if status, reason := runStatus(t, pool, run); status != "stopped" || reason != "the agent was disabled" {
		t.Errorf("run after disable: %s (%q), want stopped (the agent was disabled)", status, reason)
	}
	if _, err := svc.StartRun(ctx, name); !errors.Is(err, agentloop.ErrHalted) {
		t.Errorf("a disabled agent started a run: %v", err)
	}
	switchAgent(t, operator, name, true, "fixed")
	if err := svc.BeforeStep(ctx, run); !errors.Is(err, agentloop.ErrRunFinished) {
		t.Errorf("a stopped run took another step once its agent was enabled again: %v", err)
	}
	for action, want := range map[string]int{"platform_agent.enabled": 2, "platform_agent.disabled": 1} {
		if n := countRow(t, pool, `SELECT count(*) FROM audit_events e JOIN platform_agents a ON a.id = e.resource_id
			WHERE a.name = $1 AND e.action = $2 AND e.actor_user_id::text = $3`, name, action, operator.userID); n != want {
			t.Errorf("%s audit events by the operator: %d, want %d", action, n, want)
		}
	}
}

func TestTheAgentBrakeStopsRunsAndHoldsUntilReleased(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	const name = "agent-brake-holds"
	svc := registerTestAgent(t, pool, name)
	operator := a.login(t, "operator-agent-brake")
	a.auth.Operators = map[string]bool{operator.userID: true}
	switchAgent(t, operator, name, true, "on")
	run, err := svc.StartRun(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	releasesBefore := agentAuditCount(t, pool, "platform_agent.brake_released")

	code, brake := operatorCall(t, operator, http.MethodPut, "/admin/agents/brake", `{"note":"incident"}`)
	if code != http.StatusOK || brake["reason"] != "incident" {
		t.Fatalf("engage: got %d %v", code, brake)
	}
	if code, listed := operatorCall(t, operator, http.MethodGet, "/admin/agents", ""); code != http.StatusOK || listed["brake"] == nil {
		t.Errorf("GET /admin/agents while braked: %d %v, want the brake listed", code, listed)
	}
	if err := svc.BeforeStep(ctx, run); !errors.Is(err, agentloop.ErrHalted) {
		t.Fatalf("the next step under the brake: got %v, want ErrAgentHalted", err)
	}
	if status, reason := runStatus(t, pool, run); status != "stopped" || reason != "the agent brake is engaged" {
		t.Errorf("run under the brake: %s (%q)", status, reason)
	}
	if _, err := svc.StartRun(ctx, name); !errors.Is(err, agentloop.ErrHalted) {
		t.Errorf("an enabled agent started under the brake: %v", err)
	}

	if code, _ := operatorCall(t, operator, http.MethodDelete, "/admin/agents/brake", `{"note":"resolved"}`); code != http.StatusNoContent {
		t.Fatalf("release: got %d", code)
	}
	if _, err := svc.StartRun(ctx, name); err != nil {
		t.Errorf("an enabled agent could not start after the release: %v", err)
	}
	if code, _ := operatorCall(t, operator, http.MethodDelete, "/admin/agents/brake", `{"note":"again"}`); code != http.StatusNoContent {
		t.Errorf("releasing a released brake: got %d, want 204", code)
	}
	if n := agentAuditCount(t, pool, "platform_agent.brake_released"); n != releasesBefore+1 {
		t.Errorf("brake releases audited: %d, want %d (a release of nothing is not audited)", n, releasesBefore+1)
	}
}

func TestAgentChangesNeedANoteAndAKnownAgent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	const name = "agent-needs-note"
	registerTestAgent(t, pool, name)
	operator := a.login(t, "operator-agent-note")
	a.auth.Operators = map[string]bool{operator.userID: true}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPut, "/admin/agents/" + name + "/enabled", `{"enabled":true}`},
		{http.MethodPut, "/admin/agents/" + name + "/enabled", `{"enabled":true,"note":"   "}`},
		{http.MethodPut, "/admin/agents/" + name + "/enabled", `{"note":"no decision"}`},
		{http.MethodPut, "/admin/agents/brake", `{}`},
		{http.MethodDelete, "/admin/agents/brake", `not json`},
	} {
		if code, _ := operatorCall(t, operator, tc.method, tc.path, tc.body); code != http.StatusBadRequest {
			t.Errorf("%s %s %s: got %d, want 400", tc.method, tc.path, tc.body, code)
		}
	}
	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/agents/no-such-agent/enabled",
		`{"enabled":true,"note":"n"}`); code != http.StatusNotFound {
		t.Errorf("an unknown agent: got %d, want 404", code)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM platform_agents WHERE name = $1 AND enabled", name); n != 0 {
		t.Error("a refused request enabled the agent")
	}
}
