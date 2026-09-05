package apiserver_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// 05 R-46 (b): the acceptance criteria the person confirmed with the brief
// become a Test Case of the candidate skill, written in the same transaction
// as the version, owned by the confirming workspace.
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
	err := testPool.QueryRow(context.Background(),
		"SELECT acceptance_criteria FROM test_cases WHERE id=$1 AND skill_id=$2",
		v.Snapshot.Candidate.TestCaseID, v.Snapshot.Candidate.SkillID).Scan(&raw)
	if err != nil {
		t.Fatal(err)
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

// 05 R-46 (raise): a session refused for its budget continues once the budget
// is raised within the published band; outside the band it is refused with the
// band's own error, and steps are never bought for free.
func TestCreationRaiseBudgetLetsALimitedSessionContinue(t *testing.T) {
	a, s, _ := creationFixture(t)
	alice := a.login(t, "creation-raise")
	// The smallest budget the band allows is one call's reservation.
	v := creationPost(t, alice, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .1}, 200)
	v = creationStep(t, s, v)
	act := func(kind string, extra map[string]any, want int) map[string]any {
		body := map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind}
		for k, val := range extra {
			body[k] = val
		}
		return body
	}
	// Spent $0.01 plus the next reservation exceeds the $0.10 budget: refused.
	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("confirm_brief", nil, 422), 422)
	// Above the band: refused with the band's error, nothing changes.
	creationPost(t, alice, "/creation-sessions/"+v.ID+"/actions", act("raise_budget", map[string]any{"budget_usd": 5.0}, 422), 422)
	// Not a raise: refused too.
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
