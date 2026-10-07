package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AgentTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type AgentStepRecord struct {
	Tool      string `json:"tool"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

type AgentStepRequest struct {
	Agent           string            `json:"agent"`
	RunID           string            `json:"run_id"`
	ModelRole       string            `json:"model_role"`
	Tools           []AgentTool       `json:"tools"`
	Steps           []AgentStepRecord `json:"steps"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	GatewayKey      string            `json:"-"`
}

type AgentToolIntent struct {
	Tool      string `json:"tool"`
	Arguments string `json:"arguments"`
}

type AgentStepResponse struct {
	Outcome       string           `json:"outcome"`
	ToolIntent    *AgentToolIntent `json:"tool_intent,omitempty"`
	Result        string           `json:"result,omitempty"`
	Model         string           `json:"model"`
	PromptVersion string           `json:"prompt_version"`
	Usage         *GatewayUsage    `json:"usage,omitempty"`
}

func (c *Client) AgentStep(ctx context.Context, in AgentStepRequest) (*AgentStepResponse, error) {
	if in.GatewayKey == "" {
		return nil, fmt.Errorf("llmclient: agent gateway key is required")
	}
	ctx, cancel := withFallbackDeadline(ctx)
	defer cancel()

	in.Tools, in.Steps = nonNil(in.Tools), nonNil(in.Steps)
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal agent step: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/agent/step", bytes.NewReader(withoutHidden(body)))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create agent step request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("X-Agent-Gateway-Key", in.GatewayKey)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: agent step call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llmclient: agent step returned %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("llmclient: read agent step response: %w", err)
	}
	if len(raw) > MaxResponseBytes {
		return nil, fmt.Errorf("llmclient: agent step response exceeds %d bytes", MaxResponseBytes)
	}
	var out AgentStepResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("llmclient: decode agent step response: %w", err)
	}
	return &out, nil
}
