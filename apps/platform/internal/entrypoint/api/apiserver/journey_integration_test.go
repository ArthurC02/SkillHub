package apiserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestTheCoreJourneyRunsFromIntentSearchToADownloadedPackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "journey-curator")
	makeCatalog(t, pool, curator.workspaceID)
	catalogSkill, _ := importFiles(t, a, pool, curator, map[string]string{
		"SKILL.md": "---\nname: journey-deduplicator\n" +
			"description: Removes duplicate rows from a spreadsheet.\n---\n\nProse.\n",
		"LICENSE":      mitText,
		"reference.md": "How duplicates are decided.\n",
	})
	allowRedistribution(t, pool, catalogSkill)

	j := &journey{a: a, pool: pool, traveller: a.login(t, "journey-traveller"), catalogSkill: catalogSkill}

	if !t.Run("catalog discovery", j.discoverTheCatalogueSkill) {
		return
	}
	if !t.Run("fork and test design", j.forkAndDesignATest) {
		return
	}
	if !t.Run("run orchestration and evaluation", j.runAndEvaluate) {
		return
	}
	t.Run("package delivery", j.downloadThePackage)
}

type journey struct {
	a            *api
	pool         *pgxpool.Pool
	traveller    *client
	catalogSkill string

	forkID, forkVersionID, testCaseID, criterionID string
}

func (j *journey) discoverTheCatalogueSkill(t *testing.T) {
	traveller, catalogSkill := j.traveller, j.catalogSkill
	results := traveller.search(t, "/api/skills/search?q=remove+duplicate+rows")
	if !contains(results.ids(), catalogSkill) {
		t.Fatalf("the catalogue skill is not findable by intent: %v", results.ids())
	}

	var detail struct {
		SkillID string `json:"skill_id"`
		Name    string `json:"name"`
		Version struct {
			SkillVersionID string `json:"skill_version_id"`
		} `json:"current_version"`
	}
	if code := getJSON(t, traveller.Client,
		traveller.base+"/api/skills/"+catalogSkill, &detail); code != http.StatusOK {
		t.Fatalf("GET detail: got %d", code)
	}
	if detail.SkillID != catalogSkill {
		t.Fatalf("the detail page is a different skill: %+v", detail)
	}
}

func (j *journey) forkAndDesignATest(t *testing.T) {
	traveller, catalogSkill := j.traveller, j.catalogSkill
	code, forked := postJSON(t, traveller, "/skills/"+catalogSkill+"/fork", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, forked)
	}
	j.forkID, _ = forked["skill_id"].(string)
	if j.forkID == "" || j.forkID == catalogSkill {
		t.Fatalf("the fork is not a new skill in the caller's workspace: %v", forked)
	}
	j.forkVersionID = latestVersionID(t, j.pool, j.forkID)

	code, testCase := postJSON(t, traveller, "/test-cases",
		`{"skill_id":"`+j.forkID+`","name":"dedupe the ledger",`+
			`"user_prompt":"remove the duplicate rows from the attached ledger"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST test case: got %d, body %v", code, testCase)
	}
	j.testCaseID, _ = testCase["test_case_id"].(string)
	if j.testCaseID == "" {
		t.Fatalf("the test case has no id: %v", testCase)
	}

	code, withCriterion := postJSON(t, traveller, "/test-cases/"+j.testCaseID+"/criteria",
		`{"text":"the duplicate rows are gone"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST criterion: got %d, body %v", code, withCriterion)
	}

	criteria, _ := withCriterion["acceptance_criteria"].([]any)
	if len(criteria) != 1 {
		t.Fatalf("the test case has %d criteria: %v", len(criteria), withCriterion)
	}
	j.criterionID, _ = criteria[0].(map[string]any)["id"].(string)
	if j.criterionID == "" {
		t.Fatalf("the criterion has no id: %v", criteria[0])
	}
}

func (j *journey) runAndEvaluate(t *testing.T) {
	a, pool, traveller := j.a, j.pool, j.traveller
	f := fixture{client: traveller, skillID: j.forkID, versionID: j.forkVersionID, testCaseID: j.testCaseID}
	judge := judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: j.criterionID, Result: "passed", Reason: "the reply says the duplicates were removed",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed the duplicate rows"},
				}},
		},
		Overall: "met", Summary: "the duplicates were removed",
	}, "judge-run@2026-08-18")

	evaluator := *a.evaluations
	evaluator.Judge = judge
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1}, &evaluator)
	if a.app.Deps.Runs.Svc.Gateway == nil {
		t.Fatal("the journey HTTP handler has no model gateway")
	}

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))

	if final.CleanupStatus.Label == "" {
		t.Error("the run reports no cleanup state")
	}

	assertRunListedUnderItsSkill(t, traveller, created.RunID, j.forkID)

	waitForAutomaticEvaluation(t, pool, created.RunID)
	seedFinalOutput(t, pool, traveller.workspaceID, created.RunID,
		"Removed the duplicate rows and saved the result.")
	if err := evaluator.Evaluate(t.Context(),
		mustUUID(t, traveller.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	status, evaluation := traveller.getEvaluation(t, "/runs/"+created.RunID+"/evaluation")
	if status != http.StatusOK {
		t.Fatalf("GET evaluation: got %d (%s)", status, evaluation.Error)
	}
	if evaluation.Status != "completed" || evaluation.Overall != "met" {
		t.Fatalf("evaluation status=%q overall=%q", evaluation.Status, evaluation.Overall)
	}

	if after := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded)); after.Status !=
		final.Status {
		t.Errorf("the evaluation moved the run from %q to %q", final.Status, after.Status)
	}
}

func assertRunListedUnderItsSkill(t *testing.T, c *client, runID, skillID string) {
	t.Helper()
	var inHistory bool
	for _, r := range c.listRuns(t) {
		if r.RunID == runID {
			inHistory = true
			if r.SkillID != skillID {
				t.Errorf("the history row points at %s, not the fork %s", r.SkillID, skillID)
			}
		}
	}
	if !inHistory {
		t.Errorf("run %s finished and is not in the workspace's history", runID)
	}
}

func (j *journey) downloadThePackage(t *testing.T) {
	traveller, forkID, forkVersionID := j.traveller, j.forkID, j.forkVersionID
	var preview struct {
		Allowed       bool   `json:"allowed"`
		BlockedReason string `json:"blocked_reason"`
	}
	if code := getJSON(t, traveller.Client,
		traveller.base+packagingPath(forkID, forkVersionID)+"/preview?target=standard",
		&preview); code != http.StatusOK {
		t.Fatalf("GET packaging preview: got %d", code)
	}
	if !preview.Allowed {
		t.Fatalf("packaging the fork was refused: %s", preview.BlockedReason)
	}
	code, built := postJSON(t, traveller, packagingPath(forkID, forkVersionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, built)
	}
	artifactID, _ := built["artifact_id"].(string)

	resp, data := traveller.fetchContent(t, artifactID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET download content: got %d", resp.StatusCode)
	}

	assertAnImportableForkOf(t, data, j.catalogSkill)
	assertDownloadedOnce(t, traveller, artifactID)
	if strings.TrimSpace(string(readFromZip(t, data, "LICENSE"))) == "" {
		t.Error("the licence did not survive the journey")
	}
}

func assertAnImportableForkOf(t *testing.T, data []byte, catalogSkill string) {
	t.Helper()
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Fatalf("what the journey handed the user is not an openable package: %v", err)
	}
	report := skillpkg.Validate(fsys)
	if report.Blocked {
		t.Fatalf("the downloaded package would not import: %+v", report.Findings)
	}
	if report.Manifest == nil || report.Manifest.Name != "journey-deduplicator" {
		t.Fatalf("the downloaded package is a different skill: %+v", report.Manifest)
	}
	var manifest map[string]any
	if err := json.Unmarshal(readFromZip(t, data, "skillhub-manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}

	origin := manifest["source"].(map[string]any)["origin"].(map[string]any)
	if origin["kind"] != "fork" || origin["upstream_skill_id"] != catalogSkill {
		t.Errorf("the downloaded package does not trace back to the catalogue entry: %v", origin)
	}
}

func assertDownloadedOnce(t *testing.T, c *client, artifactID string) {
	t.Helper()
	var records struct {
		Records []map[string]any `json:"records"`
	}
	if code := getJSON(t, c.Client,
		c.base+"/downloads/"+artifactID+"/records", &records); code != http.StatusOK {
		t.Fatalf("GET download records: got %d", code)
	}
	if len(records.Records) != 1 {
		t.Errorf("%d download records after one download", len(records.Records))
	}
	found := false
	for _, d := range c.listDownloads(t) {
		if d.ArtifactID == artifactID {
			found = true
			if d.DownloadCount != 1 {
				t.Errorf("download_count = %d after one download", d.DownloadCount)
			}
		}
	}
	if !found {
		t.Error("the package the journey produced is not in the workspace's download list")
	}
}
