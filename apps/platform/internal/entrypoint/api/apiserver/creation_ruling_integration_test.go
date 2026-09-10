package apiserver_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreationMaterializeCreatesTheAcceptanceTestCase(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-criteria-alice")
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	if v.Snapshot.PendingAction != "confirm_brief" || len(v.Snapshot.AcceptanceCriteria) != 1 {
		t.Fatalf("brief proposal without criteria: %+v", v.Snapshot)
	}
	v = creationAct(t, alice, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.State != "draft_ready" {
		t.Fatalf("no draft after confirming brief and criteria: %+v", v)
	}
	v = creationAct(t, alice, v, "materialize")
	if v.Snapshot.Candidate == nil || v.Snapshot.Candidate.TestCaseID == "" {
		t.Fatalf("candidate without a test case: %+v", v.Snapshot.Candidate)
	}
	var raw []byte
	var prompt string
	err := testPool.QueryRow(context.Background(),
		"SELECT acceptance_criteria, user_prompt FROM test_cases WHERE id=$1 AND skill_id=$2",
		v.Snapshot.Candidate.TestCaseID, v.Snapshot.Candidate.SkillID).Scan(&raw, &prompt)
	if err != nil {
		t.Fatal(err)
	}

	if prompt != v.Snapshot.SampleInput || prompt == v.Snapshot.Brief {
		t.Fatalf("test case prompt is not the sample input: prompt=%q sample=%q", prompt, v.Snapshot.SampleInput)
	}
	var criteria []struct {
		Text        string     `json:"text"`
		Source      string     `json:"source"`
		ConfirmedAt *time.Time `json:"confirmed_at"`
	}
	if err := json.Unmarshal(raw, &criteria); err != nil {
		t.Fatal(err)
	}
	if len(criteria) != 1 || criteria[0].Text != "輸出摘要含所有輸入重點" || criteria[0].Source != "user" || criteria[0].ConfirmedAt == nil {
		t.Fatalf("test case does not carry the confirmed criteria: %s", raw)
	}
	bob := a.login(t, "creation-criteria-bob")
	if bob.status(t, "GET", "/test-cases/"+v.Snapshot.Candidate.TestCaseID) != 404 {
		t.Fatal("another workspace can read the creation test case")
	}
}

func TestCreationRaiseBudgetLetsALimitedSessionContinue(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-raise")

	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .1}, 200)
	v = creationStep(t, s, v)
	act := func(kind string, extra map[string]any, want int) map[string]any {
		body := map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind}
		for k, val := range extra {
			body[k] = val
		}
		return body
	}

	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("confirm_brief", nil, 422), 422)

	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("raise_budget", map[string]any{"budget_usd": 5.0}, 422), 422)

	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("raise_budget", map[string]any{"budget_usd": .1}, 422), 422)
	raised := creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("raise_budget", map[string]any{"budget_usd": .5}, 200), 200)
	if raised.Snapshot.BudgetUSD != .5 || raised.State != "waiting_confirmation" || raised.Snapshot.PendingAction != "confirm_brief" {
		t.Fatalf("raise changed more than the budget: %+v", raised)
	}
	after := creationAct(t, alice, raised, "confirm_brief")
	if after.State != "queued" {
		t.Fatalf("confirming after the raise did not queue a step: %+v", after)
	}
}

func TestCreationCandidateDoesNotCountAgainstTheSingleShotAllowance(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-quota-alice")
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, alice, v, "confirm_brief")
	v = creationStep(t, s, v)
	v = creationAct(t, alice, v, "materialize")
	if v.Snapshot.Candidate == nil {
		t.Fatal("no candidate")
	}
	ws := workspaceOf(t, testPool, alice)
	row, err := gen.New(testPool).CountGeneratedSkills(context.Background(), gen.CountGeneratedSkillsParams{WorkspaceID: ws.ID, Since: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if row.Used != 0 {
		t.Fatalf("an interactive candidate was counted against the single-shot allowance: used=%d", row.Used)
	}
	var sources int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM skill_sources WHERE workspace_id=$1 AND source_type='generated'", ws.ID).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sources != 1 {
		t.Fatalf("the candidate's source row is missing: %d", sources)
	}
}

func TestAccountPurgeRemovesCreationSessions(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-purge-alice")
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	ws := workspaceOf(t, testPool, alice)
	count := func(q string) int {
		var n int
		if err := testPool.QueryRow(context.Background(), q, ws.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("SELECT count(*) FROM creation_session_events WHERE workspace_id=$1") == 0 || count("SELECT count(*) FROM creation_receipts WHERE workspace_id=$1") == 0 {
		t.Fatal("the session left no events or receipts to purge")
	}
	if code := alice.status(t, "DELETE", "/me"); code != 200 {
		t.Fatalf("DELETE /me: %d", code)
	}
	if _, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), a.packages, 0, 10); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"creation_sessions", "creation_session_events", "creation_receipts"} {
		if n := count("SELECT count(*) FROM " + table + " WHERE workspace_id=$1"); n != 0 {
			t.Fatalf("%d %s rows survived account deletion", n, table)
		}
	}
	_ = v
}

func TestCreationMessageKeepsAConfirmedBrief(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-keep-brief")
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, alice, v, "confirm_brief")
	v = creationStep(t, s, v)
	if !v.Snapshot.BriefConfirmed {
		t.Fatal("fixture did not confirm the brief")
	}
	v = creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": "請繼續。"}, 200)
	if !v.Snapshot.BriefConfirmed || v.State != "queued" {
		t.Fatalf("a message un-confirmed the brief: confirmed=%v state=%s", v.Snapshot.BriefConfirmed, v.State)
	}
}
