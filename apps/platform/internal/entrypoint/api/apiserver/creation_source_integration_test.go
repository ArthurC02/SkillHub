package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestCreationRechecksLegacyDraftSourcesBeforeCreatingANewVersion(t *testing.T) {
	a, service, calls := creationFixture(t)
	creator := a.login(t, "creation-legacy-sources")
	view := creationPost(t, creator, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650,
	}, http.StatusOK)
	view = creationStep(t, service, view)
	view = creationAct(t, creator, view, "confirm_brief")
	view = creationStep(t, service, view)
	if view.Snapshot.Draft == nil {
		t.Fatalf("fixture did not produce a draft: %+v", view)
	}
	var encoded []byte
	if err := testPool.QueryRow(t.Context(), `SELECT snapshot FROM creation_sessions WHERE id = $1 AND workspace_id = $2`, mustUUID(t, view.ID), mustUUID(t, creator.workspaceID)).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var original map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &original); err != nil {
		t.Fatal(err)
	}
	if len(original["source_checked_hash"]) == 0 {
		t.Fatal("the verified draft's source check was never persisted")
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creation_sessions SET snapshot = snapshot - 'source_checked_hash' WHERE id = $1 AND workspace_id = $2`, mustUUID(t, view.ID), mustUUID(t, creator.workspaceID)); err != nil {
		t.Fatal(err)
	}
	view = creationAct(t, creator, view, "materialize")
	if view.State != "queued" || view.Snapshot.Draft == nil || view.Snapshot.Candidate != nil || !strings.Contains(view.Snapshot.Messages[len(view.Snapshot.Messages)-1].Content, "規則來源") {
		t.Fatalf("legacy draft bypassed source verification: %+v", view)
	}
	var skills int
	if err := testPool.QueryRow(t.Context(), "SELECT count(*) FROM skills WHERE workspace_id = $1", mustUUID(t, creator.workspaceID)).Scan(&skills); err != nil {
		t.Fatal(err)
	}
	if skills != 0 {
		t.Fatalf("source-unchecked draft created %d Skills", skills)
	}
	view = creationStep(t, service, view)
	view = creationAct(t, creator, view, "materialize")
	if view.Snapshot.Candidate == nil || calls.Load() != 3 {
		t.Fatalf("rechecked draft did not materialize once: view=%+v calls=%d", view, calls.Load())
	}
	current, err := service.Get(context.Background(), identity.Workspace{ID: mustUUID(t, creator.workspaceID)}, mustUUID(t, view.ID))
	if err != nil {
		t.Fatal(err)
	}
	assertCreationSourceCheckIsPrivate(t, current)
}

func assertCreationSourceCheckIsPrivate(t *testing.T, view creation.View) {
	t.Helper()
	public, err := json.Marshal(view)
	if err != nil || strings.Contains(string(public), "source_checked_hash") || strings.Contains(string(public), "source_assessment") {
		t.Fatalf("private source metadata leaked: %s, %v", public, err)
	}
}

func TestCreationPythonSourceInventoryReachesTheGoOwner(t *testing.T) {
	python := creationPythonExecutable(t)
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, schema, ok := readCreationGatewayPrompts(t, w, r)
		if !ok {
			return
		}
		calls.Add(1)
		if schema != "creation_decision" {
			t.Errorf("source assessment called %q, want the existing decision call", schema)
		}
		draft := creationDraft("Compute grades using author-provided weights.", "Read references/grading.md.")
		draft["files"] = []map[string]string{{"path": "references/grading.md", "content": "Math 40%, science 60%."}}
		answer := creationDecision("draft", "已依作者規則起草。", nil, draft)
		answer["source_assessment"] = map[string]any{"rules": []map[string]any{{
			"rule": "Course weights", "source_message_index": 0,
			"quote": "數學占 40%、自然占 60%。", "path": "references/grading.md", "question": nil,
		}}}
		content, err := json.Marshal(answer)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-response-cost", "0.01")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "source-proof", "object": "chat.completion", "created": 1, "model": "fixture-model",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}))
	t.Cleanup(gateway.Close)
	url := startCreationPython(t, python, gateway.URL)
	owner := creation.ModelOrNone(&llmclient.Client{BaseURL: url, Token: "test-service"})
	got, err := owner.CreationStep(t.Context(), creation.StepRequest{
		SessionID: "source-proof", Revision: 1, Brief: "Compute grades.", BriefConfirmed: true,
		Messages:       []creation.Message{{Role: "user", Content: "數學占 40%、自然占 60%。"}},
		TimeoutSeconds: 10, MaxOutputTokens: 2000, GatewayKey: "test-attempt-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceAssessment == nil || len(got.SourceAssessment.Rules) != 1 {
		t.Fatalf("the owner lost the provider inventory: %+v", got)
	}
	rule := got.SourceAssessment.Rules[0]
	messageIndex := 0
	expected := creation.SourceRule{Rule: "Course weights", SourceMessageIndex: &messageIndex, Quote: ptr("數學占 40%、自然占 60%。"), Path: ptr("references/grading.md")}
	if got.Outcome != "draft" || !reflect.DeepEqual(rule, expected) {
		t.Fatalf("the owner received different source evidence: %+v", rule)
	}
	if calls.Load() != 1 || got.Usage == nil || !got.Usage.CostReported || got.Usage.CostUSD == nil || *got.Usage.CostUSD != 0.01 {
		t.Fatalf("source inventory changed the call count or cost: calls=%d usage=%+v", calls.Load(), got.Usage)
	}
}
