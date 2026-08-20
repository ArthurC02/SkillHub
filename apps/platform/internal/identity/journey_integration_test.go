// QA-001 / RELEASE-002: one test that walks 02 §7 DoD's first journey end to
// end — intent search, detail, fork, test case, preflight, run, evaluation,
// packaging, download — through the real route table.
//
// Every one of those nine steps already had its own integration test, and this
// milestone still shipped four returns in a row (TEST-004 had no display, TEST-003
// no operation, EVAL-011 no handoff, WS-004 no listing). All four were between two
// steps. A test that exercises stages cannot see a seam by construction, which is
// why this one is a single linear function with no table and no subtests: the
// order IS the assertion.
//
// The sandbox provider is internal/run/providertest and the judge is an httptest
// server speaking llm-internal.yaml. Neither isolates nor thinks; what is under
// test is whether one stage hands the next what it needs. Real isolation is
// SEC-009's, and it is a deployment-time check.
package identity_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/eval"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run/providertest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trace"
)

func TestTheCoreJourneyRunsFromIntentSearchToADownloadedPackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	// --- what the catalogue offers -------------------------------------------
	// A real import rather than a seeded row: everything downstream reads package
	// bytes, and a fabricated object key would make packaging answer "unreadable"
	// and the journey would pass for the wrong reason.
	curator := a.login(t, "journey-curator")
	makeCatalog(t, pool, curator.workspaceID)
	catalogSkill, _ := importFiles(t, a, pool, curator, map[string]string{
		"SKILL.md": "---\nname: journey-deduplicator\n" +
			"description: Removes duplicate rows from a spreadsheet.\n---\n\nProse.\n",
		"LICENSE":      mitText,
		"reference.md": "How duplicates are decided.\n",
	})
	allowRedistribution(t, pool, catalogSkill)

	traveller := a.login(t, "journey-traveller")

	// --- 1. intent search -----------------------------------------------------
	results := traveller.search(t, "/api/skills/search?q=remove+duplicate+rows")
	if !contains(results.ids(), catalogSkill) {
		t.Fatalf("the catalogue skill is not findable by intent: %v", results.ids())
	}

	// --- 2. detail ------------------------------------------------------------
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

	// --- 3. fork --------------------------------------------------------------
	// The seam the search and detail pages hand across is the skill id, and this is
	// the first step that writes anything into the traveller's own workspace.
	code, forked := postJSON(t, traveller, "/skills/"+catalogSkill+"/fork", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, forked)
	}
	forkID, _ := forked["skill_id"].(string)
	if forkID == "" || forkID == catalogSkill {
		t.Fatalf("the fork is not a new skill in the caller's workspace: %v", forked)
	}
	forkVersionID := latestVersionID(t, pool, forkID)

	// --- 4. a test case of one's own -----------------------------------------
	code, testCase := postJSON(t, traveller, "/test-cases",
		`{"skill_id":"`+forkID+`","name":"dedupe the ledger",`+
			`"user_prompt":"remove the duplicate rows from the attached ledger"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST test case: got %d, body %v", code, testCase)
	}
	testCaseID, _ := testCase["test_case_id"].(string)
	if testCaseID == "" {
		t.Fatalf("the test case has no id: %v", testCase)
	}
	// An acceptance criterion, because a run with none is `undetermined` by
	// construction and never reaches the judge (EVAL-001). The journey's evaluation
	// step would then pass while proving nothing.
	code, withCriterion := postJSON(t, traveller, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"the duplicate rows are gone"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST criterion: got %d, body %v", code, withCriterion)
	}
	// The criterion id is the server's, not a literal: the snapshot the judge is
	// asked about is frozen from this row, and a verdict naming an id that is not in
	// it comes back `undetermined` — which is itself a seam, and one this test found
	// the first time it ran.
	criteria, _ := withCriterion["acceptance_criteria"].([]any)
	if len(criteria) != 1 {
		t.Fatalf("the test case has %d criteria: %v", len(criteria), withCriterion)
	}
	criterionID, _ := criteria[0].(map[string]any)["id"].(string)
	if criterionID == "" {
		t.Fatalf("the criterion has no id: %v", criteria[0])
	}

	// --- 5. preflight, then the run ------------------------------------------
	// The gate of 02:TEST-005: read what the run may touch, agree to it, and only
	// then create the run. The confirmed hash is the seam — a run created without
	// the hash the preflight actually produced is refused, and that refusal is what
	// makes the screen more than decoration.
	f := fixture{client: traveller, skillID: forkID, versionID: forkVersionID, testCaseID: testCaseID}

	judge := judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: criterionID, Result: "passed", Reason: "the reply says the duplicates were removed",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed the duplicate rows"},
				}},
		},
		Overall: "met", Summary: "the duplicates were removed",
	}, "judge-run@2026-08-18")
	// Wired the way cmd/worker wires it, so the evaluation the run enqueues is the
	// one that runs. Without the judge the worker records a failed evaluation, which
	// is honest and is not a verdict.
	evaluator := &eval.Service{
		Pool: pool, Store: a.packages, Judge: judge,
		// Injected, never built inside eval's own methods (ADR-032 §5).
		Trace: &trace.Service{Pool: pool},
	}
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1}, evaluator)

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))
	// ADR-025, and the reason this line is here rather than in the evaluation
	// section: `succeeded` is execution, not a task verdict, and a journey test is
	// exactly where somebody would be tempted to treat it as the finish line.
	if final.CleanupStatus == "" {
		t.Error("the run reports no cleanup state")
	}

	// The seam only a journey sees: the run just created is in the workspace's
	// history, addressed by the same platform run_id the create call returned.
	var inHistory bool
	for _, r := range traveller.listRuns(t) {
		if r.RunID == created.RunID {
			inHistory = true
			if r.SkillID != forkID {
				t.Errorf("the history row points at %s, not the fork %s", r.SkillID, forkID)
			}
		}
	}
	if !inHistory {
		t.Errorf("run %s finished and is not in the workspace's history", created.RunID)
	}

	// --- 6. the evaluation ----------------------------------------------------
	// The judge needs something to quote, and in a real run the harness emits it.
	// Seeded here because the fake provider produces no trace: what this step is
	// testing is that a finished run reaches a verdict through the queue, not what
	// a sandbox writes.
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
	// ADR-025's separation, asserted where it is easiest to lose: two resources,
	// two facts, and reading the verdict must not have moved the run.
	if after := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded)); after.Status !=
		final.Status {
		t.Errorf("the evaluation moved the run from %q to %q", final.Status, after.Status)
	}

	// --- 7. packaging ---------------------------------------------------------
	// The seam here is the licensing verdict: the fork carries the catalogue's
	// `redistribution`, and without that this step would refuse with
	// `license_unknown` (04 M4-2's own reasoning, exercised for real).
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

	// --- 8. the download ------------------------------------------------------
	resp, data := traveller.fetchContent(t, artifactID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET download content: got %d", resp.StatusCode)
	}
	// The journey ends with bytes on the user's disk, so it ends with an assertion
	// about those bytes rather than about the response code.
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
	// The last seam, and the one nothing else in this journey checks: the package
	// traces back to the catalogue entry the traveller searched for, three steps and
	// one fork ago (02:DISC-003 第 5 條).
	origin := manifest["source"].(map[string]any)["origin"].(map[string]any)
	if origin["kind"] != "fork" || origin["upstream_skill_id"] != catalogSkill {
		t.Errorf("the downloaded package does not trace back to the catalogue entry: %v", origin)
	}

	// And the download is recorded where its owner reads it (WS-004), which is the
	// step 02 §7 DoD's first sentence ends on.
	var records struct {
		Records []map[string]any `json:"records"`
	}
	if code := getJSON(t, traveller.Client,
		traveller.base+"/downloads/"+artifactID+"/records", &records); code != http.StatusOK {
		t.Fatalf("GET download records: got %d", code)
	}
	if len(records.Records) != 1 {
		t.Errorf("%d download records after one download", len(records.Records))
	}
	found := false
	for _, d := range traveller.listDownloads(t) {
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
	if strings.TrimSpace(string(readFromZip(t, data, "LICENSE"))) == "" {
		t.Error("the licence did not survive the journey")
	}
}
