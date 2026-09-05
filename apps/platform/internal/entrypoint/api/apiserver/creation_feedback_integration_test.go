package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestCreationRevisionReceivesVerifiedRunEvidence(t *testing.T) {
	a, service, _ := creationFixture(t)
	c := a.login(t, "creation-evidence-owner")
	other := a.login(t, "creation-evidence-other")
	ctx := context.Background()
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "Create a summary skill", "budget_usd": .5}, 200)
	v = creationStep(t, service, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, service, v)
	v = creationAct(t, c, v, "materialize")
	candidate := *v.Snapshot.Candidate
	oldHash := v.Snapshot.Draft.ContentHash
	wrongVersionRun, _ := seedEvaluatableRun(t, testPool, c.workspaceID, candidate.SkillID)
	var runID string
	if err := testPool.QueryRow(ctx, `INSERT INTO runs
		(workspace_id, skill_version_id, test_case_snapshot_id, provider, runtime_snapshot, policy_snapshot, status, finished_at)
		SELECT workspace_id, $2, test_case_snapshot_id, provider, runtime_snapshot, policy_snapshot, status, finished_at
		FROM runs WHERE id=$1 RETURNING id::text`, mustUUID(t, wrongVersionRun), mustUUID(t, candidate.VersionID)).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	const excerpt = "Duplicate rows remain in the input."
	const reason = "The response confirms that duplicates were not removed."
	seedFinalOutput(t, testPool, c.workspaceID, runID, excerpt)
	a.app.EvalSvc.Judge = judgeServer(t, llmclient.JudgeVerdict{
		Overall: "not_met", Summary: "Deduplication was not completed.",
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "failed", Reason: reason, EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "agent_output", Quote: excerpt}}},
			{CriterionID: "c2", Result: "undetermined", Reason: "No output artifact is available."},
		},
	}, "creation-evidence-test/v1")
	if err := a.app.EvalSvc.Evaluate(ctx, mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	path := "/creation-sessions/" + v.ID + "/actions"
	action := func(id string) map[string]any {
		return map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "attach_run", "run_id": id}
	}
	creationPost(t, c, path, action(wrongVersionRun), 404)
	creationPost(t, other, path, action(runID), 404)
	var seen atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.CreationStepRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		for _, message := range req.Messages {
			if message.Role != "tool" || !strings.Contains(message.Content, runID) {
				continue
			}
			seen.Store(true)
			for _, want := range []string{reason, excerpt, `"evaluation_available":true`, `"available":true`, `"result":"failed"`, candidate.VersionID} {
				if !strings.Contains(message.Content, want) {
					t.Errorf("observation missing %q: %s", want, message.Content)
				}
			}
		}
		if req.Draft == nil || req.DraftValidation == nil || req.DraftValidation.ContentHash != oldHash {
			t.Error("missing prior draft and its validation")
			http.Error(w, "missing draft", 500)
			return
		}
		draft := *req.Draft
		draft.Body += "\nVerify that duplicate rows were removed; report missing artifacts honestly.\n"
		cost := .01
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.CreationStepResponse{Outcome: "draft", Message: "Revised using verified evidence", Brief: req.Brief, Draft: &draft, Model: "fixture", PromptVersion: "test/v1", Usage: &llmclient.GatewayUsage{CostUSD: &cost}})
	}))
	t.Cleanup(model.Close)
	service.LLM = &llmclient.Client{BaseURL: model.URL}
	v = creationPost(t, c, path, action(runID), 200)
	v = creationStep(t, service, v)
	if !seen.Load() || v.State != "draft_ready" || v.Snapshot.Draft == nil || v.Snapshot.Draft.ContentHash == oldHash || v.Snapshot.Candidate != nil || v.Snapshot.PreviousDraft == nil || v.Snapshot.PreviousDraft.ContentHash != oldHash {
		t.Fatalf("feedback did not produce a separate revision: %+v seen=%t", v, seen.Load())
	}
	var immutableVersion string
	if err := testPool.QueryRow(ctx, "SELECT id::text FROM skill_versions WHERE id=$1", mustUUID(t, candidate.VersionID)).Scan(&immutableVersion); err != nil || immutableVersion != candidate.VersionID {
		t.Fatalf("prior candidate changed: %v", err)
	}
}
