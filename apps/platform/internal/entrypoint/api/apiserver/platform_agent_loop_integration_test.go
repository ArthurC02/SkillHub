package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

type loopScript struct {
	t         *testing.T
	answers   []func(context.Context, operations.StepRequest) (operations.StepDecision, error)
	requests  []operations.StepRequest
	issued    []float64
	revoked   []string
	costs     []operations.ModelCall
	toolCalls []string
}

func (s *loopScript) runner(svc *operations.Service) *operations.Runner {
	return &operations.Runner{
		Svc: svc,
		Step: func(ctx context.Context, req operations.StepRequest) (operations.StepDecision, error) {
			s.requests = append(s.requests, req)
			if len(s.requests) > len(s.answers) {
				s.t.Fatalf("step %d asked, only %d scripted", len(s.requests), len(s.answers))
			}
			return s.answers[len(s.requests)-1](ctx, req)
		},
		IssueKey: func(_ context.Context, _ string, budgetUSD float64, _ time.Duration) (string, error) {
			s.issued = append(s.issued, budgetUSD)
			return "sk-agent-test", nil
		},
		RevokeKey: func(_ context.Context, run string) error {
			s.revoked = append(s.revoked, run)
			return nil
		},
		RecordCost: func(_ context.Context, _ pgtype.UUID, call operations.ModelCall) {
			s.costs = append(s.costs, call)
		},
		Now: time.Now,
	}
}

func (s *loopScript) tools() []operations.Tool {
	report := operations.Tool{
		Name: "maintenance_report", Description: "facts", Parameters: map[string]any{"type": "object"},
		Run: func(_ context.Context, _ json.RawMessage) (any, error) {
			s.toolCalls = append(s.toolCalls, "maintenance_report")
			return map[string]any{"database_bytes": 42}, nil
		},
	}
	broken := operations.Tool{
		Name: "broken_tool", Description: "fails", Parameters: map[string]any{"type": "object"},
		Run: func(_ context.Context, _ json.RawMessage) (any, error) {
			s.toolCalls = append(s.toolCalls, "broken_tool")
			return nil, errors.New("facts unavailable")
		},
	}
	return []operations.Tool{report, broken}
}

func intent(tool string) func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
	cost := 0.001
	return func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
		return operations.StepDecision{
			ToolIntent: &operations.ToolCall{Tool: tool, Arguments: "{}"},
			Call:       operations.ModelCall{Model: "m", PromptTokens: 100, CompletionTokens: 10, CostUSD: &cost},
		}, nil
	}
}

func final(result string) func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
	cost := 0.002
	return func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
		return operations.StepDecision{
			Result: json.RawMessage(result),
			Call:   operations.ModelCall{Model: "m", PromptTokens: 200, CompletionTokens: 20, CostUSD: &cost},
		}, nil
	}
}

var loopLimits = operations.Limits{
	MaxSteps: 4, MaxTokens: 10_000, Deadline: time.Minute, StepTimeout: 10 * time.Second, MaxOutputTokens: 500,
}

func loopAgent(t *testing.T, name string, capMicros int64, tools ...string) (*operations.Service, operations.Definition) {
	t.Helper()
	svc, def, _ := loopAgentWithOperator(t, name, capMicros, tools...)
	return svc, def
}

func loopAgentWithOperator(t *testing.T, name string, capMicros int64, tools ...string) (*operations.Service, operations.Definition, *client) {
	t.Helper()
	pool := requireDB(t)
	a := newAPI(t, pool)
	svc := registerTestAgent(t, pool, name)
	def := operations.Definition{
		Name: name, Purpose: "loop test", ModelRole: "skillhub-test", DailySpendCapMicros: capMicros, Tools: tools,
	}
	if err := svc.Register(context.Background(), []operations.Definition{def}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM platform_agent_steps WHERE run_id IN (SELECT r.id FROM platform_agent_runs r JOIN platform_agents a ON a.id = r.agent_id WHERE a.name = $1)", name)
	})
	operator := a.login(t, "operator-"+name)
	a.auth.Operators = map[string]bool{operator.userID: true}
	switchAgent(t, operator, name, true, "loop test")
	return svc, def, operator
}

func stepRows(t *testing.T, run pgtype.UUID) []string {
	t.Helper()
	rows, err := testPool.Query(context.Background(), "SELECT tool FROM platform_agent_steps WHERE run_id = $1 ORDER BY seq", run)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tools []string
	for rows.Next() {
		var tool string
		if err := rows.Scan(&tool); err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func TestAnAgentRunCallsItsToolThenFinishesWithItsResult(t *testing.T) {
	svc, def := loopAgent(t, "agent-loop-completes", 1_000_000, "maintenance_report")
	s := &loopScript{t: t, answers: []func(context.Context, operations.StepRequest) (operations.StepDecision, error){
		intent("maintenance_report"), final(`{"summary":"all fine"}`),
	}}
	report, err := s.runner(svc).Run(context.Background(), def, s.tools(), loopLimits)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != operations.RunCompleted || string(report.Result) != `{"summary":"all fine"}` {
		t.Fatalf("report = %+v, want completed with the final result", report)
	}
	assertStoredCompletion(t, report.ID, `{"summary": "all fine"}`, []string{"maintenance_report", "finish"})
	if len(s.requests[0].Tools) != 1 || s.requests[0].Tools[0].Name != "maintenance_report" {
		t.Errorf("offered tools %v, want only the one the definition allows", s.requests[0].Tools)
	}
	if steps := s.requests[1].Steps; len(steps) != 1 || steps[0].Result != `{"database_bytes":42}` {
		t.Errorf("second step was shown %v, want the tool's encoded answer", steps)
	}
	if len(s.issued) != 1 || s.issued[0] != 1 || len(s.revoked) != 1 || len(s.costs) != 2 {
		t.Errorf("issued %v revoked %v costs %d, want one $1 key, revoked, two costs", s.issued, s.revoked, len(s.costs))
	}
}

func assertStoredCompletion(t *testing.T, run pgtype.UUID, result string, steps []string) {
	t.Helper()
	if status, _ := runStatus(t, testPool, run); status != "completed" {
		t.Errorf("stored status %s, want completed", status)
	}
	var stored string
	if err := testPool.QueryRow(context.Background(), "SELECT result::text FROM platform_agent_runs WHERE id = $1", run).Scan(&stored); err != nil || stored != result {
		t.Errorf("stored result %q (%v), want %q", stored, err, result)
	}
	if got := stepRows(t, run); !slices.Equal(got, steps) {
		t.Errorf("recorded steps %v, want %v", got, steps)
	}
}

func TestAToolErrorIsShownToTheModelAndTheRunGoesOn(t *testing.T) {
	svc, def := loopAgent(t, "agent-loop-tool-error", 1_000_000, "broken_tool")
	s := &loopScript{t: t, answers: []func(context.Context, operations.StepRequest) (operations.StepDecision, error){
		intent("broken_tool"), final(`{}`),
	}}
	report, err := s.runner(svc).Run(context.Background(), def, s.tools(), loopLimits)
	if err != nil || report.Status != operations.RunCompleted {
		t.Fatalf("report = %+v, err %v", report, err)
	}
	if steps := s.requests[1].Steps; len(steps) != 1 || steps[0].Result != `{"error":"facts unavailable"}` {
		t.Errorf("the model was shown %v, want the tool's error", steps)
	}
}

func TestAnAgentRunEndsWithoutSuccessWhenItCannotGoOn(t *testing.T) {
	never := func(ctx context.Context, _ operations.StepRequest) (operations.StepDecision, error) {
		<-ctx.Done()
		return operations.StepDecision{}, ctx.Err()
	}
	for _, tc := range []struct {
		name    string
		answers []func(context.Context, operations.StepRequest) (operations.StepDecision, error)
		limits  operations.Limits
		status  operations.RunStatus
		reason  string
		tools   []string
	}{
		{name: "a tool it was not offered", answers: answers(intent("drop_database")),
			limits: loopLimits, status: operations.RunFailed,
			reason: "operations: the model asked for a tool this agent was not offered: drop_database"},
		{name: "a result that is not JSON", answers: answers(final(`{`)),
			limits: loopLimits, status: operations.RunFailed, reason: "operations: the final result is not JSON"},
		{name: "the step limit", answers: answers(intent("maintenance_report"), intent("maintenance_report")),
			limits: withLimits(func(l *operations.Limits) { l.MaxSteps = 2 }), status: operations.RunIncomplete,
			reason: "the run reached its step limit"},
		{name: "the token limit", answers: answers(intent("maintenance_report")),
			limits: withLimits(func(l *operations.Limits) { l.MaxTokens = 110 }), status: operations.RunIncomplete,
			reason: "the run reached its token limit"},
		{name: "just under the token limit", answers: answers(intent("maintenance_report"), final(`{}`)),
			limits: withLimits(func(l *operations.Limits) { l.MaxTokens = 111 }), status: operations.RunCompleted},
		{name: "the time limit", answers: answers(never),
			limits: withLimits(func(l *operations.Limits) { l.Deadline = 200 * time.Millisecond }), status: operations.RunIncomplete,
			reason: "the run reached its time limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, def := loopAgent(t, "agent-loop-ends", 1_000_000, "maintenance_report")
			s := &loopScript{t: t, answers: tc.answers}
			report, err := s.runner(svc).Run(context.Background(), def, s.tools(), tc.limits)
			if err != nil {
				t.Fatal(err)
			}
			status, reason := runStatus(t, testPool, report.ID)
			if report.Status != tc.status || status != string(tc.status) || reason != tc.reason {
				t.Errorf("run ended %s (%q), stored %s, want %s (%q)", report.Status, reason, status, tc.status, tc.reason)
			}
			if tc.name == "a tool it was not offered" && len(s.toolCalls) != 0 {
				t.Errorf("tools ran: %v", s.toolCalls)
			}
		})
	}
}

func answers(steps ...func(context.Context, operations.StepRequest) (operations.StepDecision, error)) []func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
	return steps
}

func withLimits(change func(*operations.Limits)) operations.Limits {
	limits := loopLimits
	change(&limits)
	return limits
}

func TestTheDailySpendCapStopsARunBeforeAKeyIsIssued(t *testing.T) {
	const capMicros = 3_000
	svc, def := loopAgent(t, "agent-loop-spend-cap", capMicros, "maintenance_report")
	first := &loopScript{t: t, answers: answers(intent("maintenance_report"), final(`{}`))}
	if report, err := first.runner(svc).Run(context.Background(), def, first.tools(), loopLimits); err != nil || report.Status != operations.RunCompleted {
		t.Fatalf("first run: %+v %v", report, err)
	}
	if len(first.issued) != 1 || first.issued[0] != 0.003 {
		t.Errorf("first key budget %v, want the whole $0.003 cap", first.issued)
	}

	second := &loopScript{t: t}
	report, err := second.runner(svc).Run(context.Background(), def, second.tools(), loopLimits)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != operations.RunIncomplete || report.Reason != "the agent's daily spend cap is reached" || len(second.issued) != 0 {
		t.Errorf("second run %+v with keys %v, want incomplete at the cap and no key", report, second.issued)
	}
}

func TestDisablingAnAgentStopsItsLoopBeforeTheNextStep(t *testing.T) {
	svc, def := loopAgent(t, "agent-loop-disabled", 1_000_000, "maintenance_report")
	var operatorID pgtype.UUID
	if err := testPool.QueryRow(context.Background(), "SELECT owner_id FROM platform_agents WHERE name = $1", def.Name).Scan(&operatorID); err != nil {
		t.Fatal(err)
	}
	disableThenAsk := func(ctx context.Context, req operations.StepRequest) (operations.StepDecision, error) {
		if _, err := svc.Disable(ctx, def.Name, operatorID, "stop it"); err != nil {
			t.Fatal(err)
		}
		return intent("maintenance_report")(ctx, req)
	}
	s := &loopScript{t: t, answers: answers(disableThenAsk)}
	report, err := s.runner(svc).Run(context.Background(), def, s.tools(), loopLimits)
	if !errors.Is(err, operations.ErrAgentHalted) || report.Status != operations.RunStopped {
		t.Fatalf("report %+v err %v, want stopped by ErrAgentHalted", report, err)
	}
	if status, reason := runStatus(t, testPool, report.ID); status != "stopped" || reason != "the agent was disabled" {
		t.Errorf("stored %s (%q)", status, reason)
	}
	if len(s.requests) != 1 || len(s.revoked) != 1 {
		t.Errorf("steps asked %d, keys revoked %d; want one step and the key revoked", len(s.requests), len(s.revoked))
	}
}

func TestAnAgentsModelCallIsACostEventWithNoUserAndNoDebit(t *testing.T) {
	svc, def := loopAgent(t, "agent-loop-cost", 1_000_000, "maintenance_report")
	ctx := context.Background()
	run, err := svc.StartRun(ctx, def.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.FinishRun(ctx, run, operations.RunCompleted, ""); err != nil {
		t.Fatal(err)
	}
	credits, err := wiring.NewCreditService(testPool)
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.0025
	wiring.AgentCostRecorder(testPool, credits)(ctx, run, operations.ModelCall{
		Model: "served-model", PromptVersion: "t1", PromptTokens: 30, CompletionTokens: 7, CostUSD: &cost,
	})

	var kind, refType, model string
	var micros, debits int64
	var userID, workspaceID pgtype.UUID
	if err := testPool.QueryRow(ctx, `SELECT kind, ref_type, model, usd_micros, user_id, workspace_id,
		(SELECT count(*) FROM credit_entries ce WHERE ce.cost_event_id = c.id)
		FROM cost_events c WHERE ref_id = $1`, run).Scan(&kind, &refType, &model, &micros, &userID, &workspaceID, &debits); err != nil {
		t.Fatal(err)
	}
	if kind != "platform_agent" || refType != "platform_agent_run" || model != "served-model" || micros != 2500 {
		t.Errorf("cost event %s/%s/%s/%d, want platform_agent/platform_agent_run/served-model/2500", kind, refType, model, micros)
	}
	if userID.Valid || workspaceID.Valid || debits != 0 {
		t.Errorf("user %v workspace %v debits %d, want none of them", userID.Valid, workspaceID.Valid, debits)
	}
}

func TestARunWhoseResultFailsItsCheckFailsAndKeepsTheResultForTheOperator(t *testing.T) {
	cases := []struct {
		name   string
		result string
		status operations.RunStatus
	}{
		{"a cite to a returned fact", `{"items":[{"status":"fine","text":"ok","cites":["/database_bytes"]}]}`, operations.RunCompleted},
		{"a cite to a fact no tool returned", `{"items":[{"status":"fine","text":"ok","cites":["/cpu_percent"]}]}`, operations.RunFailed},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, def := loopAgent(t, fmt.Sprintf("agent-loop-check-%d", i), 1_000_000, "maintenance_report")
			def.CheckResult = operations.CitesOnlyReturnedFacts
			s := &loopScript{t: t, answers: answers(intent("maintenance_report"), final(tc.result))}
			report, err := s.runner(svc).Run(context.Background(), def, s.tools(), loopLimits)
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != tc.status {
				t.Fatalf("status %s (%s), want %s", report.Status, report.Reason, tc.status)
			}
			var stored, reason string
			var kept bool
			if err := testPool.QueryRow(context.Background(),
				"SELECT status, coalesce(reason, ''), result IS NOT NULL FROM platform_agent_runs WHERE id = $1", report.ID,
			).Scan(&stored, &reason, &kept); err != nil || stored != string(tc.status) || !kept {
				t.Fatalf("stored status %q result kept %v (%v), want %s with the result", stored, kept, err, tc.status)
			}
			if tc.status == operations.RunFailed && !strings.Contains(reason, `"/cpu_percent"`) {
				t.Errorf("reason %q does not name the cite no tool returned", reason)
			}
			if got := stepRows(t, report.ID); !slices.Equal(got, []string{"maintenance_report", "finish"}) {
				t.Errorf("recorded steps %v, want the tool call and the finish", got)
			}
		})
	}
}

func TestOperatorsReadAnAgentRunWithItsStepsAndWhatEachCost(t *testing.T) {
	svc, def, operator := loopAgentWithOperator(t, "agent-run-records", 1_000_000, "maintenance_report")
	unpriced := func(context.Context, operations.StepRequest) (operations.StepDecision, error) {
		return operations.StepDecision{Result: json.RawMessage(`{"summary":"ok"}`), Call: operations.ModelCall{Model: "m"}}, nil
	}
	s := &loopScript{t: t, answers: answers(intent("maintenance_report"), unpriced)}
	report, err := s.runner(svc).Run(context.Background(), def, s.tools(), loopLimits)
	if err != nil || report.Status != operations.RunCompleted {
		t.Fatalf("report %+v, err %v", report, err)
	}
	id := pgconv.UUIDString(report.ID)
	assertListedRun(t, operator, id, map[string]any{
		"agent": def.Name, "status": "completed", "steps": float64(2),
		"usd_micros": float64(1000), "unpriced_steps": float64(1),
	})
	assertRunSteps(t, operator, id)
}

func assertListedRun(t *testing.T, operator *client, id string, want map[string]any) {
	t.Helper()
	code, body := operatorCall(t, operator, http.MethodGet, "/admin/agents/runs", "")
	if code != http.StatusOK {
		t.Fatalf("GET runs: %d %v", code, body)
	}
	run := findByID(t, body["runs"], id)
	for key, value := range want {
		if run[key] != value {
			t.Errorf("run %s = %v, want %v", key, run[key], value)
		}
	}
	if result, _ := run["result"].(map[string]any); result["summary"] != "ok" || run["finished_at"] == nil {
		t.Errorf("run result %v finished_at %v, want the final result and a finish time", run["result"], run["finished_at"])
	}
}

func assertRunSteps(t *testing.T, operator *client, id string) {
	t.Helper()
	code, body := operatorCall(t, operator, http.MethodGet, "/admin/agents/runs/"+id+"/steps", "")
	steps, _ := body["steps"].([]any)
	if code != http.StatusOK || len(steps) != 2 {
		t.Fatalf("GET steps: %d %v, want two steps", code, body)
	}
	first, last := steps[0].(map[string]any), steps[1].(map[string]any)
	if first["tool"] != "maintenance_report" || first["result"] != `{"database_bytes":42}` ||
		first["usd_micros"] != float64(1000) || first["prompt_tokens"] != float64(100) {
		t.Errorf("first step %v, want the tool call with its answer, tokens and cost", first)
	}
	if _, priced := last["usd_micros"]; last["tool"] != "finish" || last["arguments"] != `{"summary":"ok"}` || priced {
		t.Errorf("last step %v, want finish carrying the result and no cost", last)
	}
}

func TestReadingStepsNeedsARunIDAndAnUnknownRunHasNone(t *testing.T) {
	_, _, operator := loopAgentWithOperator(t, "agent-run-records-ids", 1_000_000)
	if code, body := operatorCall(t, operator, http.MethodGet, "/admin/agents/runs/not-a-uuid/steps", ""); code != http.StatusBadRequest {
		t.Errorf("malformed id: %d %v, want 400", code, body)
	}
	code, body := operatorCall(t, operator, http.MethodGet, "/admin/agents/runs/00000000-0000-0000-0000-000000000000/steps", "")
	if steps, ok := body["steps"].([]any); code != http.StatusOK || !ok || len(steps) != 0 {
		t.Errorf("unknown run: %d %v, want 200 with an empty list", code, body)
	}
}

func findByID(t *testing.T, list any, id string) map[string]any {
	t.Helper()
	items, _ := list.([]any)
	for _, item := range items {
		if row, _ := item.(map[string]any); row["id"] == id {
			return row
		}
	}
	t.Fatalf("run %s not in %v", id, list)
	return nil
}
