package creation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func listedTools(n int) []MCPTool {
	tools := make([]MCPTool, n)
	for i := range tools {
		tools[i] = MCPTool{Reference: fmt.Sprintf("mcp__server__tool_%d", i)}
	}
	return tools
}

func TestAStepOffersTheListedMCPToolsUpToTheContractCap(t *testing.T) {
	for _, tc := range []struct {
		name   string
		listed int
		want   int
	}{
		{"empty list", 0, 0},
		{"at the cap", 64, 64},
		{"one past the cap", 65, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := DraftRules{OfferedMCPTools: func(context.Context) ([]MCPTool, error) { return listedTools(tc.listed), nil }}
			got, err := rules.offeredMCPTools(context.Background())
			if err != nil {
				t.Fatalf("reading the offered tools: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("offered %d tools, want %d", len(got), tc.want)
			}
			if tc.want > 0 && got[0].Reference != "mcp__server__tool_0" {
				t.Errorf("first offered tool = %q, want the list's first entry", got[0].Reference)
			}
		})
	}
}

func TestWithoutAToolListWiredAStepOffersNoMCPTool(t *testing.T) {
	got, err := DraftRules{}.offeredMCPTools(context.Background())
	if err != nil || got != nil {
		t.Errorf("offered = %v, %v; want nothing and no error", got, err)
	}
}

func TestAStepThatCannotReadTheToolListStopsBeforeTheModelIsCalled(t *testing.T) {
	unreadable := errors.New("tool list unreadable")
	called := false
	s := &Service{
		LLM: modelFunc(func(context.Context, StepRequest) (*StepResult, error) {
			called = true
			return &StepResult{}, nil
		}),
		IssueKey:  func(context.Context, string, string, float64, time.Duration) (string, error) { return "key", nil },
		RevokeKey: func(context.Context, string) error { return nil },
		Drafts:    DraftRules{OfferedMCPTools: func(context.Context) ([]MCPTool, error) { return nil, unreadable }},
	}

	err := s.Step(context.Background(), JobArgs{}, nil)

	if !errors.Is(err, unreadable) {
		t.Errorf("step error = %v, want the tool list read failure", err)
	}
	if called {
		t.Error("the model was called with an incomplete tool list")
	}
}
