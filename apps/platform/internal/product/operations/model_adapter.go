package operations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

const (
	outcomeToolIntent = "tool_intent"
	outcomeFinal      = "final"
)

func StepThrough(client *llmclient.Client) func(context.Context, StepRequest) (StepDecision, error) {
	return func(ctx context.Context, req StepRequest) (StepDecision, error) {
		resp, err := client.AgentStep(ctx, stepRequest(req))
		if err != nil {
			return StepDecision{}, err
		}
		return stepDecision(resp)
	}
}

func stepRequest(req StepRequest) llmclient.AgentStepRequest {
	tools := make([]llmclient.AgentTool, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = llmclient.AgentTool{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters}
	}
	steps := make([]llmclient.AgentStepRecord, len(req.Steps))
	for i, step := range req.Steps {
		steps[i] = llmclient.AgentStepRecord{Tool: step.Tool, Arguments: step.Arguments, Result: step.Result}
	}
	return llmclient.AgentStepRequest{
		Agent: req.Agent, RunID: req.RunID, ModelRole: req.ModelRole, Tools: tools, Steps: steps,
		TimeoutSeconds: int(req.Timeout.Seconds()), MaxOutputTokens: req.MaxOutputTokens, GatewayKey: req.GatewayKey,
	}
}

func stepDecision(resp *llmclient.AgentStepResponse) (StepDecision, error) {
	decision := StepDecision{Call: ModelCall{Model: resp.Model, PromptVersion: resp.PromptVersion}}
	if u := resp.Usage; u != nil {
		decision.Call.PromptTokens, decision.Call.CompletionTokens = u.PromptTokens, u.CompletionTokens
		decision.Call.CostUSD = u.ReportedCostUSD()
	}
	switch {
	case resp.Outcome == outcomeToolIntent && resp.ToolIntent != nil:
		decision.ToolIntent = &ToolCall{Tool: resp.ToolIntent.Tool, Arguments: resp.ToolIntent.Arguments}
	case resp.Outcome == outcomeFinal:
		decision.Result = json.RawMessage(resp.Result)
	default:
		return decision, fmt.Errorf("operations: the step answered %q without what that outcome needs", resp.Outcome)
	}
	return decision, nil
}
