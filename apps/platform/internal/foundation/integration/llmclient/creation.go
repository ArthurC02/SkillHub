package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreationStepRequest is the bounded proposal request. GatewayKey is a
// short-lived credential and deliberately has no JSON representation.
type CreationStepRequest struct {
	SessionID            string                   `json:"session_id"`
	Revision             int64                    `json:"revision"`
	Messages             []CreationMessage        `json:"messages"`
	Brief                string                   `json:"brief"`
	BriefConfirmed       bool                     `json:"brief_confirmed"`
	DiagramUnderstanding string                   `json:"diagram_understanding"`
	DiagramConfirmed     bool                     `json:"diagram_confirmed"`
	Diagram              *GenerateDiagram         `json:"diagram,omitempty"`
	References           []GenerateReference      `json:"references"`
	Draft                *GeneratedSkill          `json:"draft,omitempty"`
	DraftValidation      *CreationDraftValidation `json:"draft_validation,omitempty"`
	AllowedTools         []string                 `json:"allowed_tools"`
	TimeoutSeconds       int                      `json:"timeout_seconds"`
	MaxOutputTokens      int                      `json:"max_output_tokens"`
	GatewayKey           string                   `json:"-"`
}
type CreationDraftValidation struct {
	ContentHash string `json:"content_hash"`
	Blocked     bool   `json:"blocked"`
	Report      string `json:"report"`
}
type CreationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type CreationToolIntent struct {
	Kind  string `json:"kind"`
	Query string `json:"query"`
}
type CreationStepResponse struct {
	Outcome              string              `json:"outcome"`
	Message              string              `json:"message"`
	Brief                string              `json:"brief"`
	DiagramUnderstanding string              `json:"diagram_understanding"`
	ToolIntent           *CreationToolIntent `json:"tool_intent,omitempty"`
	Draft                *GeneratedSkill     `json:"draft,omitempty"`
	Model                string              `json:"model"`
	PromptVersion        string              `json:"prompt_version"`
	Usage                *GatewayUsage       `json:"usage,omitempty"`
}

// CreationStep uses both the service bearer and the single-session gateway key.
// The latter is supplied only in this header and is never persisted or logged.
func (c *Client) CreationStep(ctx context.Context, in CreationStepRequest) (*CreationStepResponse, error) {
	if in.GatewayKey == "" {
		return nil, fmt.Errorf("llmclient: creation gateway key is required")
	}
	// These fields are required arrays in the internal contract; nil would encode
	// as null and is not a valid empty conversation or reference set.
	if in.Messages == nil {
		in.Messages = []CreationMessage{}
	}
	if in.References == nil {
		in.References = []GenerateReference{}
	}
	if in.AllowedTools == nil {
		in.AllowedTools = []string{}
	}
	if in.Draft != nil {
		draft := *in.Draft
		if draft.Files == nil {
			draft.Files = []GeneratedFile{}
		}
		in.Draft = &draft
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal creation step: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/creation/step", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create creation step request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("X-Creation-Gateway-Key", in.GatewayKey)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: creation step call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llmclient: creation step returned %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("llmclient: read creation step response: %w", err)
	}
	if len(raw) > MaxResponseBytes {
		return nil, fmt.Errorf("llmclient: creation step response exceeds %d bytes", MaxResponseBytes)
	}
	var out CreationStepResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("llmclient: decode creation step response: %w", err)
	}
	return &out, nil
}
