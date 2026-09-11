package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func post[Req, Resp any](ctx context.Context, c *Client, path string, reqBody Req) (*Resp, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal %s request: %w", path, err)
	}

	body = withoutHidden(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("llmclient: %s call: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("llmclient: %s returned %d: %s", path, resp.StatusCode, string(b))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("llmclient: read %s response: %w", path, err)
	}
	if len(raw) > MaxResponseBytes {
		return nil, fmt.Errorf("llmclient: %s response exceeds %d bytes", path, MaxResponseBytes)
	}
	var result Resp
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("llmclient: decode %s response: %w", path, err)
	}
	return &result, nil
}

const MaxResponseBytes = 8 << 20

type EmbedRequest struct {
	Texts []string `json:"texts"`

	TimeoutSeconds float64 `json:"timeout_seconds,omitempty"`
}

type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`

	Usage *GatewayUsage `json:"usage,omitempty"`
}

func (c *Client) Embed(ctx context.Context, texts []string) (*EmbedResponse, error) {
	return post[EmbedRequest, EmbedResponse](ctx, c, "/embed", EmbedRequest{Texts: texts})
}

func (c *Client) EmbedWithin(ctx context.Context, texts []string, seconds float64) (*EmbedResponse, error) {
	req := EmbedRequest{Texts: texts}
	if seconds > 0 {
		req.TimeoutSeconds = seconds
	}
	return post[EmbedRequest, EmbedResponse](ctx, c, "/embed", req)
}

type TaskExample struct {
	ZhHant string `json:"zh_hant"`
	En     string `json:"en"`
}

type SkillTags struct {
	Inputs       []string `json:"inputs"`
	Outputs      []string `json:"outputs"`
	Tools        []string `json:"tools"`
	Dependencies []string `json:"dependencies"`
}

type EnrichSkillRequest struct {
	SkillName string   `json:"skill_name"`
	SkillMD   string   `json:"skill_md"`
	FileTree  []string `json:"file_tree,omitempty"`
	Language  string   `json:"language,omitempty"`
}

type EnrichSkillResponse struct {
	Summary      string        `json:"summary"`
	TaskExamples []TaskExample `json:"task_examples"`
	Tags         SkillTags     `json:"tags"`

	Limitations   []string `json:"limitations"`
	Model         string   `json:"model"`
	PromptVersion string   `json:"prompt_version"`

	Checks []EnrichCheck `json:"checks,omitempty"`

	Usage *GatewayUsage `json:"usage,omitempty"`
}

type EnrichCheck struct {
	Rule     string `json:"rule"`
	Field    string `json:"field"`
	Token    string `json:"token,omitempty"`
	Severity string `json:"severity,omitempty"`
}

func (c *Client) EnrichSkill(ctx context.Context, req EnrichSkillRequest) (*EnrichSkillResponse, error) {
	return post[EnrichSkillRequest, EnrichSkillResponse](ctx, c, "/v1/enrich-skill", req)
}

type SkillCandidate struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type MatchReasonsRequest struct {
	Query      string           `json:"query"`
	Candidates []SkillCandidate `json:"candidates"`
}

type MatchReason struct {
	SkillID string `json:"skill_id"`
	Reason  string `json:"reason"`
}

type MatchReasonsResponse struct {
	Reasons []MatchReason `json:"reasons"`
	Model   string        `json:"model"`

	Usage *GatewayUsage `json:"usage,omitempty"`
}

func (c *Client) MatchReasons(ctx context.Context, query string, candidates []SkillCandidate) (*MatchReasonsResponse, error) {
	return post[MatchReasonsRequest, MatchReasonsResponse](ctx, c, "/match-reasons",
		MatchReasonsRequest{Query: query, Candidates: candidates})
}

type DatasetField struct {
	Name         string `json:"name"`
	InferredType string `json:"inferred_type"`
}

type DatasetOutline struct {
	FileName    string         `json:"file_name"`
	ContentType string         `json:"content_type,omitempty"`
	Fields      []DatasetField `json:"fields,omitempty"`
}

type SuggestCriteriaRequest struct {
	SkillName    string           `json:"skill_name,omitempty"`
	SkillSummary string           `json:"skill_summary,omitempty"`
	UserPrompt   string           `json:"user_prompt"`
	Datasets     []DatasetOutline `json:"datasets,omitempty"`
}

type SuggestedCriterion struct {
	Text string `json:"text"`
}

type SuggestCriteriaResponse struct {
	Criteria []SuggestedCriterion `json:"criteria"`
}

func (c *Client) SuggestCriteria(ctx context.Context, req SuggestCriteriaRequest) (*SuggestCriteriaResponse, error) {
	return post[SuggestCriteriaRequest, SuggestCriteriaResponse](ctx, c, "/suggest-criteria", req)
}

type JudgeCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	EvidenceExcerpt string `json:"evidence_excerpt,omitempty"`
}

type JudgeArtifact struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type,omitempty"`
	TextExcerpt string `json:"text_excerpt,omitempty"`
}

type TraceDigestEntry struct {
	TraceEventID string `json:"trace_event_id"`
	OccurredAt   string `json:"occurred_at"`
	Type         string `json:"type"`
	Excerpt      string `json:"excerpt"`
}

type TraceDigest struct {
	Complete bool               `json:"complete"`
	Entries  []TraceDigestEntry `json:"entries"`
}

type RubricItem struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	Weight           *float64 `json:"weight,omitempty"`
	EvidenceRequired bool     `json:"evidence_required"`
}

type Rubric struct {
	Items []RubricItem `json:"items"`
}

type JudgeSkill struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type JudgeRunRequest struct {
	RunID        string           `json:"run_id"`
	EvaluationID string           `json:"evaluation_id"`
	Skill        *JudgeSkill      `json:"skill,omitempty"`
	UserPrompt   string           `json:"user_prompt"`
	Criteria     []JudgeCriterion `json:"criteria"`
	Rubric       *Rubric          `json:"rubric,omitempty"`
	FinalOutput  string           `json:"final_output"`
	Artifacts    []JudgeArtifact  `json:"artifacts"`
	TraceDigest  TraceDigest      `json:"trace_digest"`

	Truncation []string `json:"truncation"`
}

type JudgeEvidenceRef struct {
	Kind         string  `json:"kind"`
	TraceEventID *string `json:"trace_event_id"`
	ArtifactPath *string `json:"artifact_path"`
	Quote        string  `json:"quote"`
}

type CriterionVerdict struct {
	CriterionID  string             `json:"criterion_id"`
	Result       string             `json:"result"`
	Reason       string             `json:"reason"`
	EvidenceRefs []JudgeEvidenceRef `json:"evidence_refs"`
}

type JudgeVerdict struct {
	CriterionResults []CriterionVerdict `json:"criterion_results"`
	Overall          string             `json:"overall"`
	Summary          string             `json:"summary"`
}

type GatewayUsage struct {
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
	CostSource       string   `json:"cost_source"`
}

type JudgeUsage = GatewayUsage

type JudgeRunResponse struct {
	Verdict       JudgeVerdict `json:"verdict"`
	Model         string       `json:"model"`
	PromptVersion string       `json:"prompt_version"`
	Usage         *JudgeUsage  `json:"usage,omitempty"`
}

func (c *Client) JudgeRun(ctx context.Context, req JudgeRunRequest) (*JudgeRunResponse, error) {
	return post[JudgeRunRequest, JudgeRunResponse](ctx, c, "/judge-run", req)
}

type TargetFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type SuggestImprovementsRequest struct {
	EvaluationID     string       `json:"evaluation_id"`
	EvaluationDigest string       `json:"evaluation_digest"`
	FileTree         []string     `json:"file_tree,omitempty"`
	TargetFiles      []TargetFile `json:"target_files,omitempty"`
}

type ImprovementProposal struct {
	Category string `json:"category"`
	Problem  string `json:"problem"`

	Evidence        string `json:"evidence"`
	TargetPath      string `json:"target_path"`
	ProposedContent string `json:"proposed_content"`
	ExpectedImpact  string `json:"expected_impact"`
}

type SuggestImprovementsResponse struct {
	Suggestions   []ImprovementProposal `json:"suggestions"`
	Model         string                `json:"model"`
	PromptVersion string                `json:"prompt_version"`
	Usage         *GatewayUsage         `json:"usage,omitempty"`
}

func (c *Client) SuggestImprovements(
	ctx context.Context, req SuggestImprovementsRequest,
) (*SuggestImprovementsResponse, error) {
	return post[SuggestImprovementsRequest, SuggestImprovementsResponse](
		ctx, c, "/suggest-improvements", req)
}

type GenerateSkillRequest struct {
	TaskDescription string              `json:"task_description,omitempty"`
	Diagram         *GenerateDiagram    `json:"diagram,omitempty"`
	References      []GenerateReference `json:"references,omitempty"`
}

type GenerateDiagram struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type GenerateReference struct {
	Name    string `json:"name"`
	SkillMD string `json:"skill_md"`
}

type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type GeneratedSkill struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Compatibility string          `json:"compatibility"`
	AllowedTools  string          `json:"allowed_tools"`
	Body          string          `json:"body"`
	Files         []GeneratedFile `json:"files"`
}

type GenerateSkillResponse struct {
	Skill         GeneratedSkill `json:"skill"`
	Model         string         `json:"model"`
	PromptVersion string         `json:"prompt_version"`
	Usage         *GatewayUsage  `json:"usage,omitempty"`
}

var ErrGenerateTruncated = errors.New("llmclient: generated skill was truncated at the token ceiling")

// truncationMarker must match apps/llm's 502 detail text verbatim; matching
// only a substring like "truncated" would also fire on unrelated errors that
// happen to reuse the word.
const truncationMarker = "generate model output was truncated at the token ceiling"

func (c *Client) GenerateSkill(ctx context.Context, req GenerateSkillRequest) (*GenerateSkillResponse, error) {
	resp, err := post[GenerateSkillRequest, GenerateSkillResponse](ctx, c, "/v1/generate-skill", req)
	if err != nil && strings.Contains(err.Error(), truncationMarker) {
		return nil, fmt.Errorf("%w: %v", ErrGenerateTruncated, err)
	}
	return resp, err
}
