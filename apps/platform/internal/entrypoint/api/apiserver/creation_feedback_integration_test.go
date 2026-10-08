package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestCreationRevisionReceivesVerifiedRunEvidence(t *testing.T) {
	a, service, _ := creationFixture(t)
	c := a.login(t, "creation-evidence-owner")
	other := a.login(t, "creation-evidence-other")
	ctx := context.Background()
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "Create a summary skill", "budget_credits": 650}, 200)
	v = creationStep(t, service, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, service, v)
	v = creationAct(t, c, v, "materialize")
	candidate := *v.Snapshot.Candidate
	oldHash := v.Snapshot.Draft.ContentHash
	wrongVersionRun, _ := seedEvaluatableRun(t, testPool, c.workspaceID, candidate.SkillID)
	var runID string
	if err := testPool.QueryRow(ctx, `WITH r AS (
			INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, status, finished_at)
			SELECT workspace_id, $2, test_case_snapshot_id, provider, status, finished_at
			FROM runs WHERE id=$1 RETURNING id, workspace_id),
		s AS (
			INSERT INTO run_snapshots (run_id, workspace_id, runtime_snapshot, policy_snapshot)
			SELECT r.id, r.workspace_id, o.runtime_snapshot, o.policy_snapshot
			FROM r, run_snapshots o WHERE o.run_id=$1)
		SELECT id::text FROM r`, mustUUID(t, wrongVersionRun), mustUUID(t, candidate.VersionID)).Scan(&runID); err != nil {
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
	creationPost(t, c, path, action("not-a-run"), 404)
	var runningID string
	if err := testPool.QueryRow(ctx, `WITH r AS (
			INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, status)
			SELECT workspace_id, skill_version_id, test_case_snapshot_id, provider, 'running'
			FROM runs WHERE id=$1 RETURNING id, workspace_id),
		s AS (
			INSERT INTO run_snapshots (run_id, workspace_id, runtime_snapshot, policy_snapshot)
			SELECT r.id, r.workspace_id, o.runtime_snapshot, o.policy_snapshot
			FROM r, run_snapshots o WHERE o.run_id=$1)
		SELECT id::text FROM r`, mustUUID(t, runID)).Scan(&runningID); err != nil {
		t.Fatal(err)
	}
	creationPost(t, c, path, action(runningID), 422)
	evidence := &revisionEvidence{
		runID: runID, priorHash: oldHash,
		mustMention: []string{reason, excerpt, `"evaluation_available":true`, `"available":true`, `"result":"failed"`, candidate.VersionID},
	}
	seen := &evidence.seen
	model := httptest.NewServer(evidence.revisingModel(t))
	t.Cleanup(model.Close)
	service.LLM = creation.ModelOrNone(&llmclient.Client{BaseURL: model.URL})
	v = creationPost(t, c, path, action(runID), 200)

	if v.State != "waiting_input" || !strings.Contains(v.Snapshot.Messages[len(v.Snapshot.Messages)-1].Content, reason) {
		t.Fatalf("an unmet trial must ask the person before the model revises: state=%q last=%+v", v.State, v.Snapshot.Messages[len(v.Snapshot.Messages)-1])
	}
	v = creationMessage(t, c, v, "照沒過的條件改草稿。")
	v = creationStep(t, service, v)
	if !seen.Load() || v.State != "draft_ready" || v.Snapshot.Draft == nil || v.Snapshot.Draft.ContentHash == oldHash || v.Snapshot.Candidate != nil || v.Snapshot.PreviousDraft == nil || v.Snapshot.PreviousDraft.ContentHash != oldHash {
		t.Fatalf("feedback did not produce a separate revision: %+v seen=%t", v, seen.Load())
	}
	var immutableVersion string
	if err := testPool.QueryRow(ctx, "SELECT id::text FROM skill_versions WHERE id=$1", mustUUID(t, candidate.VersionID)).Scan(&immutableVersion); err != nil || immutableVersion != candidate.VersionID {
		t.Fatalf("prior candidate changed: %v", err)
	}
}

type revisionEvidence struct {
	runID, priorHash string
	mustMention      []string
	seen             atomic.Bool
}

func (e *revisionEvidence) assertObservationsMentionTheRun(t *testing.T, messages []llmclient.CreationMessage) {
	for _, message := range messages {
		if message.Role != "tool" || !strings.Contains(message.Content, e.runID) {
			continue
		}
		e.seen.Store(true)
		for _, want := range e.mustMention {
			if !strings.Contains(message.Content, want) {
				t.Errorf("observation missing %q: %s", want, message.Content)
			}
		}
	}
}

func (e *revisionEvidence) revisingModel(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.CreationStepRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		e.assertObservationsMentionTheRun(t, req.Messages)
		if req.Draft == nil || req.DraftValidation == nil || req.DraftValidation.ContentHash != e.priorHash {
			t.Error("missing prior draft and its validation")
			http.Error(w, "missing draft", http.StatusInternalServerError)
			return
		}
		draft := *req.Draft
		draft.Body += "\nVerify that duplicate rows were removed; report missing artifacts honestly.\n"
		cost := .01
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.CreationStepResponse{Outcome: "draft", Message: "Revised using verified evidence", Brief: req.Brief, Draft: &draft, Model: "fixture", PromptVersion: "test/v1", Usage: &llmclient.GatewayUsage{CostUSD: &cost, CostSource: llmclient.CostSourceGateway}})
	}
}
