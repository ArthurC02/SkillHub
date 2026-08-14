// Package llmclient provides a typed HTTP client for the internal Python LLM
// service (ADR-016: Python is capability provider, Go owns policy and retry).
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client calls the internal LLM service endpoints.
type Client struct {
	BaseURL    string       // e.g. "http://localhost:8001"
	HTTPClient *http.Client // uses http.DefaultClient if nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// EmbedRequest is the request body for POST /embed.
type EmbedRequest struct {
	Texts []string `json:"texts"`
}

// EmbedResponse is the response body from POST /embed.
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`
}

// Embed calls the LLM service to generate embeddings for the given texts.
func (c *Client) Embed(ctx context.Context, texts []string) (*EmbedResponse, error) {
	body, err := json.Marshal(EmbedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: embed call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llmclient: embed returned %d: %s", resp.StatusCode, string(b))
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("llmclient: decode embed response: %w", err)
	}
	return &result, nil
}

// SkillCandidate represents a search hit sent for match-reason generation.
type SkillCandidate struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// MatchReasonsRequest is the request body for POST /match-reasons.
type MatchReasonsRequest struct {
	Query      string           `json:"query"`
	Candidates []SkillCandidate `json:"candidates"`
}

// MatchReason is a single match reason returned by the LLM service.
type MatchReason struct {
	SkillID string `json:"skill_id"`
	Reason  string `json:"reason"`
}

// MatchReasonsResponse is the response body from POST /match-reasons.
type MatchReasonsResponse struct {
	Reasons []MatchReason `json:"reasons"`
}

// MatchReasons calls the LLM service to generate human-readable match reasons.
func (c *Client) MatchReasons(ctx context.Context, query string, candidates []SkillCandidate) (*MatchReasonsResponse, error) {
	body, err := json.Marshal(MatchReasonsRequest{
		Query:      query,
		Candidates: candidates,
	})
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal match-reasons request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/match-reasons", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create match-reasons request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: match-reasons call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llmclient: match-reasons returned %d: %s", resp.StatusCode, string(b))
	}

	var result MatchReasonsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("llmclient: decode match-reasons response: %w", err)
	}
	return &result, nil
}
