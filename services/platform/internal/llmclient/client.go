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

// post sends req as JSON to path and decodes the response into Resp. The error
// text names the endpoint, so a failure reads the same as the hand-rolled
// version it replaced.
func post[Req, Resp any](ctx context.Context, c *Client, path string, reqBody Req) (*Resp, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal %s request: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: %s call: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llmclient: %s returned %d: %s", path, resp.StatusCode, string(b))
	}

	var result Resp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("llmclient: decode %s response: %w", path, err)
	}
	return &result, nil
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
	return post[EmbedRequest, EmbedResponse](ctx, c, "/embed", EmbedRequest{Texts: texts})
}

// TaskExample is one example task sentence, given in both catalogue languages.
type TaskExample struct {
	ZhHant string `json:"zh_hant"`
	En     string `json:"en"`
}

// SkillTags are the input/output/tool/dependency tags the enrichment extracts.
type SkillTags struct {
	Inputs       []string `json:"inputs"`
	Outputs      []string `json:"outputs"`
	Tools        []string `json:"tools"`
	Dependencies []string `json:"dependencies"`
}

// EnrichSkillRequest is the request body for POST /v1/enrich-skill.
type EnrichSkillRequest struct {
	SkillName string   `json:"skill_name"`
	SkillMD   string   `json:"skill_md"`
	FileTree  []string `json:"file_tree,omitempty"`
	Language  string   `json:"language,omitempty"`
}

// EnrichSkillResponse is the ADR-013 enrichment whitelist plus its provenance.
// Model and PromptVersion are stored alongside the text so the projection can
// label it as model-generated and find stale enrichments to rebuild.
type EnrichSkillResponse struct {
	Summary      string        `json:"summary"`
	TaskExamples []TaskExample `json:"task_examples"`
	Tags         SkillTags     `json:"tags"`
	// Limitations restate what the document says the skill does not do or needs
	// (DISC-003 限制). A restatement, like Summary — the service does not judge
	// risk, safety or quality, and the scan-derived half of the detail view's
	// limitations list is assembled separately in catalog.
	Limitations   []string `json:"limitations"`
	Model         string   `json:"model"`
	PromptVersion string   `json:"prompt_version"`
}

// EnrichSkill runs the ADR-013 index-time enhancement for one skill version.
// Called once per version at import time; retry policy is the caller's
// (iron rule 6).
func (c *Client) EnrichSkill(ctx context.Context, req EnrichSkillRequest) (*EnrichSkillResponse, error) {
	return post[EnrichSkillRequest, EnrichSkillResponse](ctx, c, "/v1/enrich-skill", req)
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
	return post[MatchReasonsRequest, MatchReasonsResponse](ctx, c, "/match-reasons",
		MatchReasonsRequest{Query: query, Candidates: candidates})
}
