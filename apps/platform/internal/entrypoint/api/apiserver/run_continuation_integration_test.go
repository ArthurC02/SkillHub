package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

const askingPrompt = "summarise the attached csv"

func seedAskingRun(t *testing.T, pool *pgxpool.Pool, f fixture, questions ...string) string {
	t.Helper()
	ctx := context.Background()
	var runID string
	if err := pool.QueryRow(ctx, `
		WITH snap AS (
			INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
			VALUES ($1, $3, $4, '[{"id":"c1","text":"asks first","check":{"kind":"asks"}}]'::jsonb, 'sha256:asking')
			RETURNING id),
		r AS (
			INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, status, started_at, finished_at)
			SELECT $1, $2, snap.id, 'fake_sandbox', 'succeeded', now() - interval '1 minute', now() FROM snap
			RETURNING id, workspace_id),
		s AS (INSERT INTO run_snapshots (run_id, workspace_id) SELECT id, workspace_id FROM r)
		SELECT id::text FROM r`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, f.testCaseID), askingPrompt,
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if len(questions) > 0 {
		seedQuestions(t, pool, f.workspaceID, runID, questions...)
	}
	return runID
}

func seedQuestions(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string, questions ...string) {
	t.Helper()
	asked := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		asked = append(asked, map[string]any{"question": q, "why": "it changes the answer"})
	}
	payload, err := json.Marshal(map[string]any{"kind": "question", "text": "asked", "truncated": false, "questions": asked})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type,
		                          source, status, schema_version, masked, masked_fields, payload)
		VALUES (gen_random_uuid(), $1, $2, 1, 1, now(), 'agent_output', 'sandbox', 'ok',
		        '1.0', true, '[]'::jsonb, $3)`,
		mustUUID(t, workspaceID), mustUUID(t, runID), payload,
	); err != nil {
		t.Fatal(err)
	}
}

func seedAnsweredRound(t *testing.T, pool *pgxpool.Pool, f fixture, asking string, questions ...string) string {
	t.Helper()
	next := seedAskingRun(t, pool, f, questions...)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO run_continuations (run_id, workspace_id, continues_run_id, questions, answers)
		VALUES ($1, $2, $3, '[{"question":"q","why":"w"}]'::jsonb, '["a"]'::jsonb)`,
		mustUUID(t, next), mustUUID(t, f.workspaceID), mustUUID(t, asking)); err != nil {
		t.Fatal(err)
	}
	return next
}

func resetRunSettings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM run_settings`) })
}

func TestAnsweringARunsQuestionsStartsTheRunThatContinuesIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-continue")
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？", "幾點出發？")

	code, view := f.postJSON(t, "/runs/"+asking+"/continuations", `{"answers":[" 算 ","早上八點"]}`)
	if code != http.StatusCreated || view.Status != "queued" || view.RunID == asking {
		t.Fatalf("continue: got %d %+v", code, view)
	}

	var prompt, continues, questions, answers string
	if err := pool.QueryRow(context.Background(), `
		SELECT s.user_prompt, c.continues_run_id::text, c.questions::text, c.answers::text
		FROM runs r
		JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
		JOIN run_continuations c ON c.run_id = r.id
		WHERE r.id = $1`, mustUUID(t, view.RunID)).Scan(&prompt, &continues, &questions, &answers); err != nil {
		t.Fatal(err)
	}
	const wantPrompt = askingPrompt + "\n\n你先前提出的問題與我的回答：" +
		"\n1. 問：小孩算不算人數？\n   答：算" +
		"\n2. 問：幾點出發？\n   答：早上八點"
	if prompt != wantPrompt {
		t.Errorf("prompt = %q\nwant %q", prompt, wantPrompt)
	}
	if continues != asking || answers != `["算", "早上八點"]` {
		t.Errorf("link = %s answers = %s", continues, answers)
	}

	if code, _ := f.postJSON(t, "/runs/"+asking+"/continuations", `{"answers":["算","早上八點"]}`); code != http.StatusConflict {
		t.Errorf("answering the same questions twice: got %d, want 409", code)
	}
}

func TestAnswersMustMatchTheQuestionsAndOnlyAnAskingRunCanBeAnswered(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-continue-refused")
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？")
	silent := seedAskingRun(t, pool, f)

	cases := []struct {
		name, runID, body string
		want              int
	}{
		{"one answer too many", asking, `{"answers":["算","多的"]}`, http.StatusBadRequest},
		{"no answer", asking, `{"answers":[]}`, http.StatusBadRequest},
		{"a blank answer", asking, `{"answers":["  "]}`, http.StatusBadRequest},
		{"a run that never asked", silent, `{"answers":["算"]}`, http.StatusConflict},
	}
	for _, c := range cases {
		if code, _ := f.postJSON(t, "/runs/"+c.runID+"/continuations", c.body); code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, code, c.want)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM run_continuations WHERE workspace_id = $1`,
		mustUUID(t, f.workspaceID)).Scan(&n); err != nil || n != 0 {
		t.Errorf("refused answers left %d continuations (%v)", n, err)
	}
}

func TestAConversationStopsAtTheOperatorsRoundLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunSettings(t, pool)
	operator := a.login(t, freshName("operator-rounds"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/settings/runs/continuation-rounds",
		`{"rounds":1,"note":"one round while we watch costs"}`); code != http.StatusOK {
		t.Fatalf("set rounds: %d %v", code, body)
	}

	f := newFixture(t, a, pool, "alice-continue-limit")
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？")
	first := seedAnsweredRound(t, pool, f, asking, "大人幾位？")

	if code, _ := f.postJSON(t, "/runs/"+first+"/continuations", `{"answers":["兩位"]}`); code != http.StatusUnprocessableEntity {
		t.Errorf("a second round past a limit of one: got %d, want 422", code)
	}
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/settings/runs/continuation-rounds",
		`{"rounds":2,"note":"one more round"}`); code != http.StatusOK {
		t.Fatalf("raise rounds: %d %v", code, body)
	}
	if code, view := f.postJSON(t, "/runs/"+first+"/continuations", `{"answers":["兩位"]}`); code != http.StatusCreated {
		t.Errorf("the second round within a limit of two: got %d (%s)", code, view.Error)
	}
}

func TestAnOperatorSetsTheContinuationRoundsWithinTheirBounds(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunSettings(t, pool)
	operator := a.login(t, freshName("operator-run-settings"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	const path = "/admin/settings/runs/continuation-rounds"

	code, body := operatorCall(t, operator, http.MethodGet, "/admin/settings/runs", "")
	if rounds := body["continuation_rounds"].(map[string]any); code != http.StatusOK ||
		rounds["rounds"] != float64(3) || rounds["reason"] != nil {
		t.Fatalf("default: %d %v, want 3 rounds that nobody set", code, body)
	}

	refusesEach(t, operator, path, map[string]string{
		"zero":            `{"rounds":0,"note":"n"}`,
		"one past ten":    `{"rounds":11,"note":"n"}`,
		"no rounds":       `{"note":"n"}`,
		"a blank note":    `{"rounds":2,"note":"  "}`,
		"rounds as words": `{"rounds":"two","note":"n"}`,
	})

	for _, rounds := range []string{"1", "10"} {
		code, body := operatorCall(t, operator, http.MethodPut, path, `{"rounds":`+rounds+`,"note":"bounds"}`)
		if got := body["continuation_rounds"].(map[string]any)["rounds"]; code != http.StatusOK || got != mustFloat(t, rounds) {
			t.Errorf("rounds %s: %d %v", rounds, code, body)
		}
	}

	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events WHERE action = $1 AND actor_user_id = $2`,
		audit.ActionRunSettingsSet, mustUUID(t, operator.userID)).Scan(&n); err != nil || n != 2 {
		t.Errorf("audit events = %d (%v), want one per accepted change", n, err)
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTheConversationIsEvaluatedAtItsAnswerNotAtItsQuestion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-continue-eval")
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？")
	answered := seedAnsweredRound(t, pool, f, asking)
	seedFinalOutput(t, pool, f.workspaceID, answered, "算人數，共四位。")
	ctx := context.Background()

	for _, runID := range []string{asking, answered} {
		if err := a.evaluations.Evaluate(ctx, mustUUID(t, f.workspaceID), mustUUID(t, runID)); err != nil {
			t.Fatalf("evaluate %s: %v", runID, err)
		}
	}

	var waiting int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evaluations WHERE run_id = $1`, mustUUID(t, asking)).Scan(&waiting); err != nil || waiting != 0 {
		t.Errorf("the run waiting for its answer has %d evaluations (%v), want none", waiting, err)
	}
	status, body := f.getEvaluation(t, "/runs/"+answered+"/evaluation")
	if status != http.StatusOK || body.Status != "completed" || len(body.CriterionResults) != 1 {
		t.Fatalf("answered run: %d %+v", status, body)
	}
	if r := body.CriterionResults[0]; r.Source != "rule" || r.Result != "passed" {
		t.Errorf("asks-first on the answered run = %+v, want passed by the rule from the earlier round", r)
	}
}
