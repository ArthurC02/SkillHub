package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func TestCriteriaWithChecksAreDecidedWithoutAJudge(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-rule-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "dedupe-ruled")
	runID, _ := seedEvaluatableRunWithCriteria(t, pool, c.workspaceID, skillID, `[
		{"id":"c1","text":"reports how many rows were removed","source":"user",
		 "check":{"kind":"contains_values","values":["17"]}},
		{"id":"c2","text":"answers without asking","source":"user","check":{"kind":"does_not_ask"}}]`, nil)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")
	a.evaluations.Judge = nil

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	status, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if status != http.StatusOK || body.Status != "completed" || body.Overall != "met" {
		t.Fatalf("got %d status=%q overall=%q (%s)", status, body.Status, body.Overall, body.Error)
	}
	if len(body.CriterionResults) != 2 {
		t.Fatalf("results = %+v", body.CriterionResults)
	}
	for _, r := range body.CriterionResults {
		if r.Source != "rule" || r.Result != "passed" {
			t.Errorf("criterion %s = %s from %s, want passed from the rule", r.CriterionID, r.Result, r.Source)
		}
	}
	if body.JudgeModel != "" {
		t.Errorf("no model judged this run, got judge model %q", body.JudgeModel)
	}
}

func TestOnlyCriteriaWithoutAChecksReachTheJudge(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-mixed-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "dedupe-mixed")
	runID, _ := seedEvaluatableRunWithCriteria(t, pool, c.workspaceID, skillID, `[
		{"id":"c1","text":"reports 18 rows removed","source":"user",
		 "check":{"kind":"contains_values","values":["18"]}},
		{"id":"c2","text":"an xlsx file is produced","source":"user"}]`, nil)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")

	var sent llmclient.JudgeRunRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: llmclient.JudgeVerdict{
				CriterionResults: []llmclient.CriterionVerdict{
					{CriterionID: "c2", Result: "passed", Reason: "output.xlsx is in the manifest",
						EvidenceRefs: []llmclient.JudgeEvidenceRef{
							{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")},
						}},
				},
				Overall: "met", Summary: "the file was produced",
			},
			Model: "gpt-6-sol", PromptVersion: "judge-run@test",
		})
	}))
	t.Cleanup(srv.Close)
	a.evaluations.Judge = eval.JudgeOrNone(&llmclient.Client{BaseURL: srv.URL})

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if len(sent.Criteria) != 1 || sent.Criteria[0].ID != "c2" {
		t.Errorf("the judge was sent %+v, want only c2", sent.Criteria)
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if body.Overall != "partially_met" || len(body.CriterionResults) != 2 {
		t.Fatalf("overall=%q results=%+v", body.Overall, body.CriterionResults)
	}
	first, second := body.CriterionResults[0], body.CriterionResults[1]
	if first.CriterionID != "c1" || first.Source != "rule" || first.Result != "failed" {
		t.Errorf("c1 = %+v, want failed from the rule", first)
	}
	if second.CriterionID != "c2" || second.Source != "model" || second.Result != "passed" {
		t.Errorf("c2 = %+v, want passed from the model", second)
	}
}

func TestACriterionCanBeAddedWithACheckAndABadCheckIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "criterion-check-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "criterion-check")
	testCaseID := seedTestCase(t, pool, c.workspaceID, skillID)

	status, body := c.doJSON(t, http.MethodPost, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"asks about children","check":{"kind":"asks","values":[" 小孩 "]}}`)
	if status != http.StatusCreated {
		t.Fatalf("add: got %d %v", status, body)
	}
	criteria, _ := body["acceptance_criteria"].([]any)
	added, _ := criteria[len(criteria)-1].(map[string]any)
	check, _ := added["check"].(map[string]any)
	values, _ := check["values"].([]any)
	if check["kind"] != "asks" || len(values) != 1 || values[0] != "小孩" {
		t.Errorf("stored criterion = %v", added)
	}

	if status, _ := c.doJSON(t, http.MethodPost, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"must contain something","check":{"kind":"contains_values"}}`); status != http.StatusBadRequest {
		t.Errorf("a contains_values check with no values: got %d, want 400", status)
	}
}
