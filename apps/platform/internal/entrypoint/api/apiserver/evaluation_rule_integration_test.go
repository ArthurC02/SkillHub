package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestACompositeCheckIsStoredNormalizedAndAMalformedTreeIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "criterion-tree-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "criterion-tree")
	testCaseID := seedTestCase(t, pool, c.workspaceID, skillID)

	status, body := c.doJSON(t, http.MethodPost, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"reports 17 and reads well","check":{"kind":"sequence","children":[
			{"kind":"contains_values","values":[" 17 "]},{"kind":"judge"}]}}`)
	if status != http.StatusCreated {
		t.Fatalf("add: got %d %v", status, body)
	}
	criteria, _ := body["acceptance_criteria"].([]any)
	added, _ := criteria[len(criteria)-1].(map[string]any)
	check, _ := added["check"].(map[string]any)
	children, _ := check["children"].([]any)
	if check["kind"] != "sequence" || len(children) != 2 {
		t.Fatalf("stored check = %v", check)
	}
	gate, _ := children[0].(map[string]any)
	if values, _ := gate["values"].([]any); len(values) != 1 || values[0] != "17" {
		t.Errorf("stored gate = %v, want its value trimmed", gate)
	}

	if status, _ := c.doJSON(t, http.MethodPost, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"branches on nothing","check":{"kind":"if","children":[{"kind":"asks"}]}}`); status != http.StatusBadRequest {
		t.Errorf("an if with only its condition: got %d, want 400", status)
	}
}

func TestTheCheckKindsAnEditorCanOfferAreListed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "check-kinds-reader")

	status, body := c.doJSON(t, http.MethodGet, "/evaluation/check-kinds", "")
	if status != http.StatusOK {
		t.Fatalf("got %d %v", status, body)
	}
	kinds, _ := body["kinds"].([]any)
	got := map[string]string{}
	for _, k := range kinds {
		kind, _ := k.(map[string]any)
		name, _ := kind["kind"].(string)
		category, _ := kind["category"].(string)
		got[name] = category
	}
	want := map[string]string{
		"contains_values": "grader", "asks": "grader", "does_not_ask": "grader", "judge": "grader", "prompt_grader": "grader",
		"sequence": "composite", "all": "composite", "any": "composite", "if": "composite", "each": "composite",
	}
	if len(got) != len(want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
	for name, category := range want {
		if got[name] != category {
			t.Errorf("kind %s = %q, want %q", name, got[name], category)
		}
	}
	if body["max_nodes"] != float64(eval.MaxCheckNodes) || body["max_depth"] != float64(eval.MaxCheckDepth) {
		t.Errorf("limits = %v / %v", body["max_nodes"], body["max_depth"])
	}
}

func countingJudge(t *testing.T, verdict llmclient.CriterionVerdict) (eval.Judge, *[]llmclient.JudgeRunRequest) {
	t.Helper()
	var sent []llmclient.JudgeRunRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.JudgeRunRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sent = append(sent, req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: llmclient.JudgeVerdict{CriterionResults: []llmclient.CriterionVerdict{verdict}, Overall: verdict.Result},
			Model:   "gpt-6-sol", PromptVersion: "judge-run@test",
		})
	}))
	t.Cleanup(srv.Close)
	return eval.JudgeOrNone(&llmclient.Client{BaseURL: srv.URL}), &sent
}

const gatedByAValue = `[{"id":"c1","text":"reports %s rows removed and reads well","source":"user",
	"check":{"kind":"sequence","children":[{"kind":"contains_values","values":["%s"]},{"kind":"judge"}]}}]`

func TestAJudgeBehindAFailingGateIsNeverCalled(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-gate-closed")
	skillID := seedSkill(t, pool, c.workspaceID, "gate-closed")
	runID, _ := seedEvaluatableRunWithCriteria(t, pool, c.workspaceID, skillID, fmt.Sprintf(gatedByAValue, "18", "18"), nil)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")
	judge, sent := countingJudge(t, llmclient.CriterionVerdict{CriterionID: "c1", Result: "passed", Reason: "reads well"})
	a.evaluations.Judge = judge

	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if len(*sent) != 0 {
		t.Errorf("the judge was called %d times behind a failing gate", len(*sent))
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if len(body.CriterionResults) != 1 {
		t.Fatalf("results = %+v", body.CriterionResults)
	}
	r := body.CriterionResults[0]
	if r.Result != "failed" || r.Source != "rule" || len(r.Steps) != 2 || r.Steps[0].Kind != "contains_values" {
		t.Errorf("c1 = %+v, want failed by the rule with the gate's step recorded", r)
	}
}

func TestAJudgeBehindAPassingGateDecidesTheCriterion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-gate-open")
	skillID := seedSkill(t, pool, c.workspaceID, "gate-open")
	runID, _ := seedEvaluatableRunWithCriteria(t, pool, c.workspaceID, skillID, fmt.Sprintf(gatedByAValue, "17", "17"), nil)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")
	judge, sent := countingJudge(t, llmclient.CriterionVerdict{CriterionID: "c1", Result: "failed", Reason: "the tone is curt",
		EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")}}})
	a.evaluations.Judge = judge

	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if len(*sent) != 1 || len((*sent)[0].Criteria) != 1 || (*sent)[0].Criteria[0].ID != "c1" {
		t.Fatalf("judge requests = %+v, want one asking about c1", *sent)
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	r := body.CriterionResults[0]
	if r.Result != "failed" || r.Source != "model" || len(r.Steps) != 3 {
		t.Errorf("c1 = %+v, want the judge's failure labelled model with every step recorded", r)
	}
}
