package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestACreationStepShowsTheModelEveryListedMCPToolAndNoRetiredOne(t *testing.T) {
	a, service, _ := creationFixture(t)
	q := gen.New(testPool)
	listed, retired := uniqueServer(), uniqueServer()+"-old"
	if _, err := q.InsertMCPTool(context.Background(), gen.InsertMCPToolParams{
		Server: listed, Tool: "create_event", Purpose: "Creates an event in the team calendar.",
		Parameters: "title, start", ServerUrl: "https://mcp.example.com/" + listed, Reason: "creation offer check",
	}); err != nil {
		t.Fatal(err)
	}
	old, err := q.InsertMCPTool(context.Background(), gen.InsertMCPToolParams{
		Server: retired, Tool: "create_event", Purpose: "Old calendar.", ServerUrl: "https://mcp.example.com/" + retired, Reason: "creation offer check",
	})
	if err != nil {
		t.Fatal(err)
	}
	note := "replaced"
	if _, err := q.RetireMCPTool(context.Background(), gen.RetireMCPToolParams{ID: old.ID, RetireReason: &note}); err != nil {
		t.Fatal(err)
	}
	var offered []llmclient.CreationMCPTool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.CreationStepRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		offered = req.MCPTools
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.CreationStepResponse{Outcome: "clarification", Message: "Which calendar?", Model: "fixture", PromptVersion: "test/v1"})
	}))
	t.Cleanup(model.Close)
	service.LLM = creation.ModelOrNone(&llmclient.Client{BaseURL: model.URL})
	c := a.login(t, "creation-mcp-offer-owner")

	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "Book a follow-up meeting", "budget_credits": 650}, 200)
	creationStep(t, service, v)

	want := llmclient.CreationMCPTool{Reference: "mcp__" + listed + "__create_event", Purpose: "Creates an event in the team calendar.", Parameters: "title, start"}
	found := false
	for _, tool := range offered {
		found = found || tool == want
		if tool.Reference == "mcp__"+retired+"__create_event" {
			t.Errorf("the retired tool %s was offered", tool.Reference)
		}
	}
	if !found {
		t.Errorf("offered tools = %+v, want %+v among them", offered, want)
	}
}
