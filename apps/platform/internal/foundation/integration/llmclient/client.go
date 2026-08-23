// Package llmclient provides a typed HTTP client for the internal Python LLM
// service (ADR-016: Python is capability provider, Go owns policy and retry).
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

// Client calls the internal LLM service endpoints.
type Client struct {
	BaseURL    string       // e.g. "http://localhost:8001"
	Token      string       // static service credential (ADR-020)
	HTTPClient *http.Client // uses http.DefaultClient if nil
}

// httpClient deliberately returns a client with no Timeout of its own: the
// deadline is the caller's ctx and nothing else (iron rule 7 - Go owns timeout
// policy, and it owns it in one place).
//
// It used to be `&http.Client{Timeout: 30 * time.Second}`, which was shorter
// than every caller's ctx budget - ingest/enrich.go allows 75s so the LLM
// service's own 60s ceiling surfaces as its error rather than as our deadline.
// The 30s fired first, so that budget was never reachable and three of fourteen
// enrichment calls in the M1 fix round died at 30s and succeeded on a re-run.
// Worse, giving up client-side does not cancel the call upstream: every one of
// those timeouts was still billed at the gateway. A second, shorter, invisible
// deadline is not a safety net, it is a bill for work that gets thrown away.
//
// Every caller carries a deadline today (enrich 75s, embed 20s, catalog embed
// 10s / match reasons 8s, testlab suggest, generate 130s); a future one that
// forgets will hang on its own ctx, which is the failure the caller can see and
// fix. Generate is on that list because it was the one that forgot: this comment
// was the record of who had a deadline, and a record with a hole in it reads
// exactly like a record with none.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
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
		return nil, fmt.Errorf("llmclient: %s returned %d: %s", path, resp.StatusCode, string(b))
	}

	// Bounded, because "internal" is not "trusted". 02:GEN-003 says so in as many
	// words about the generation endpoint — 「平台的模型不是可信來源」 — and the
	// upload path already puts a MaxBytesReader at its own trust boundary
	// (ingest/http.go). Without this the only ceiling on a generated package was
	// MAX_OUTPUT_TOKENS, a constant in a different language: raise it, point
	// GENERATE_SKILL_MODEL at a model with a bigger cap, or compromise apps/llm,
	// and the API process buffers the JSON, the decoded structs and then a zip.
	var result Resp
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("llmclient: decode %s response: %w", path, err)
	}
	return &result, nil
}

// MaxResponseBytes caps one internal LLM response.
//
// 8 MiB: comfortably above the largest thing any current endpoint returns (a
// 16000-token generation is ~64 KB of JSON; an embedding batch is the other
// candidate and is far smaller), and far below anything that threatens the API
// process. A response that hits it fails to decode, which is the right outcome —
// a truncated JSON document is not a partial answer to be salvaged.
const MaxResponseBytes = 8 << 20

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

// DatasetField is one column of an uploaded file, as the suggestion request
// describes it. Name and inferred type only — there is deliberately no field for
// a cell value, so a caller cannot ship a user's rows to a model by accident
// (iron rule 11; the same shape is re-stated in the service's own schema).
type DatasetField struct {
	Name         string `json:"name"`
	InferredType string `json:"inferred_type"`
}

// DatasetOutline is one attached file, described by its shape.
type DatasetOutline struct {
	FileName    string         `json:"file_name"`
	ContentType string         `json:"content_type,omitempty"`
	Fields      []DatasetField `json:"fields,omitempty"`
}

// SuggestCriteriaRequest is the request body for POST /suggest-criteria.
type SuggestCriteriaRequest struct {
	SkillName    string           `json:"skill_name,omitempty"`
	SkillSummary string           `json:"skill_summary,omitempty"`
	UserPrompt   string           `json:"user_prompt"`
	Datasets     []DatasetOutline `json:"datasets,omitempty"`
}

// SuggestedCriterion is one proposed acceptance condition.
type SuggestedCriterion struct {
	Text string `json:"text"`
}

// SuggestCriteriaResponse is the response body from POST /suggest-criteria.
type SuggestCriteriaResponse struct {
	Criteria []SuggestedCriterion `json:"criteria"`
}

// SuggestCriteria asks the LLM service to propose acceptance criteria (TEST-002).
// A proposal only: what is done with it — stored as `suggested`, edited, deleted —
// is Go's decision (iron rule 6), and ctx carries the caller's timeout and
// cancellation into the internal call (iron rule 7).
func (c *Client) SuggestCriteria(ctx context.Context, req SuggestCriteriaRequest) (*SuggestCriteriaResponse, error) {
	return post[SuggestCriteriaRequest, SuggestCriteriaResponse](ctx, c, "/suggest-criteria", req)
}

// --- /judge-run (EVAL-001, the task-effect leg) -------------------------------
//
// Wire types for contracts/openapi/llm-internal.yaml. Five of EVAL-001's six
// problem classes never reach here: they are decided in Go from the platform's own
// records (evaluation-design §2.5). What crosses this boundary is one question —
// were the acceptance criteria met — and one answer, which internal/eval then
// re-verifies reference by reference before storing any of it (ADR-026 defence 3).

// JudgeCriterion is one acceptance criterion from the run's frozen snapshot.
type JudgeCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// EvidenceExcerpt is material Go already located for this criterion, masked
	// and cut at the §6.3 budget. Absent means Go found nothing specific — not
	// that nothing happened, and never grounds for `failed` on its own.
	EvidenceExcerpt string `json:"evidence_excerpt,omitempty"`
}

// JudgeArtifact is one row of the artifact manifest. Manifest only: the bytes stay
// in the archive and nothing is unpacked or parsed by extension (iron rule 1).
type JudgeArtifact struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type,omitempty"`
	TextExcerpt string `json:"text_excerpt,omitempty"`
}

// TraceDigestEntry is one already-masked trace entry the judge may cite. The
// (id, occurred_at) pair is the address: trace_events is partitioned by time, so
// the timestamp is part of it rather than decoration.
type TraceDigestEntry struct {
	TraceEventID string `json:"trace_event_id"`
	OccurredAt   string `json:"occurred_at"`
	Type         string `json:"type"`
	Excerpt      string `json:"excerpt"`
}

// TraceDigest is an aggregate, never the event stream: the LLM service has no way
// to ask for a full trace (ADR-009 low-privilege read). Entries are also the
// citation set — a trace_event reference may only name an id that appeared here.
type TraceDigest struct {
	Complete bool               `json:"complete"`
	Entries  []TraceDigestEntry `json:"entries"`
}

// RubricItem is one CONTENT-007 rubric line. Weight lives here and nowhere else.
type RubricItem struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	Weight           *float64 `json:"weight,omitempty"`
	EvidenceRequired bool     `json:"evidence_required"`
}

// Rubric strengthens the acceptance criteria; it is not a second mechanism.
type Rubric struct {
	Items []RubricItem `json:"items"`
}

// JudgeSkill is the skill under test, for context only. Untrusted package text.
type JudgeSkill struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// JudgeRunRequest is everything the judge sees, and the boundary of it.
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
	// Truncation names the fields that were cut. Non-empty means a criterion that
	// depends on a cut field may only be answered `undetermined`.
	Truncation []string `json:"truncation"`
}

// JudgeEvidenceRef is a claim about where an answer came from. It is a claim and
// not a fact: this type carries no byte or character ranges because the model does
// not compute them — Go locates `Quote` in the source it names, which is both the
// lookup and the verification (evaluation-design §2.4 defence 3).
type JudgeEvidenceRef struct {
	Kind         string  `json:"kind"`
	TraceEventID *string `json:"trace_event_id"`
	ArtifactPath *string `json:"artifact_path"`
	Quote        string  `json:"quote"`
}

// CriterionVerdict is one criterion, one answer. There is deliberately no `source`
// field: Go labels everything from this endpoint `source: model` (02:EVAL-001
// clause 5), and letting a model call its own output a rule check is not a power
// worth handing out.
type CriterionVerdict struct {
	CriterionID  string             `json:"criterion_id"`
	Result       string             `json:"result"`
	Reason       string             `json:"reason"`
	EvidenceRefs []JudgeEvidenceRef `json:"evidence_refs"`
}

// JudgeVerdict is the model-authored part of the answer, and only that.
type JudgeVerdict struct {
	CriterionResults []CriterionVerdict `json:"criterion_results"`
	Overall          string             `json:"overall"`
	Summary          string             `json:"summary"`
}

// JudgeUsage is what the call cost at the gateway, when the service reports it.
// Additive and optional: the contract's JudgeRunResponse does not require it, and
// a nil here means unreported — which the caller must not render as zero
// (same rule as the trace usage event's cost_usd).
type GatewayUsage struct {
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
	CostSource       string   `json:"cost_source"`
}

type JudgeUsage = GatewayUsage

// JudgeRunResponse separates what the model wrote (`Verdict`) from what the
// service knows and the model cannot influence (`Model`, `PromptVersion`).
type JudgeRunResponse struct {
	Verdict       JudgeVerdict `json:"verdict"`
	Model         string       `json:"model"`
	PromptVersion string       `json:"prompt_version"`
	Usage         *JudgeUsage  `json:"usage,omitempty"`
}

// JudgeRun asks the LLM service to judge one run against its acceptance criteria.
// A verdict, never a decision: retry, timeout and the fallback on failure are all
// Go's (iron rule 6/7), and ctx carries the deadline and cancellation into the
// internal call. A failure here is an evaluation failure, never a guessed pass.
func (c *Client) JudgeRun(ctx context.Context, req JudgeRunRequest) (*JudgeRunResponse, error) {
	return post[JudgeRunRequest, JudgeRunResponse](ctx, c, "/judge-run", req)
}

// --- /suggest-improvements (EVAL-002) -----------------------------------------
//
// Proposals, not changes. Nothing that comes back from here reaches a package
// until internal/eval has re-checked it and a user has accepted it, and even then
// it becomes a new Skill Version rather than an edit (iron rule 4).

// TargetFile is one package file the caller is willing to have replaced. Sent as
// content because a proposal has to be written against what is actually there;
// capped by the contract at 60,000 characters per file.
type TargetFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// SuggestImprovementsRequest is the evaluation's findings plus the files that
// might need changing. EvaluationDigest is Go's own rendering of the verdict, not
// the stored row — the same low-privilege reading rule the trace digest follows.
type SuggestImprovementsRequest struct {
	EvaluationID     string       `json:"evaluation_id"`
	EvaluationDigest string       `json:"evaluation_digest"`
	FileTree         []string     `json:"file_tree,omitempty"`
	TargetFiles      []TargetFile `json:"target_files,omitempty"`
}

// ImprovementProposal is one proposed change: 02:EVAL-002 clause 1's five fields
// and nothing decision-shaped. There is deliberately no `decision` and no
// `applied_skill_version_id` — whether a proposal is taken, and what version it
// becomes, are the user's and Go's (iron rule 6).
type ImprovementProposal struct {
	Category string `json:"category"`
	Problem  string `json:"problem"`
	// Evidence is prose the model quotes from the digest. A claim, not a citation:
	// the stored EvidenceRefs are minted by Go from the evaluation's own verified
	// references (see eval/suggest.go).
	Evidence        string `json:"evidence"`
	TargetPath      string `json:"target_path"`
	ProposedContent string `json:"proposed_content"`
	ExpectedImpact  string `json:"expected_impact"`
}

// SuggestImprovementsResponse separates model output (`Suggestions`) from what the
// service knows and the model cannot influence (`Model`, `PromptVersion`).
type SuggestImprovementsResponse struct {
	Suggestions   []ImprovementProposal `json:"suggestions"`
	Model         string                `json:"model"`
	PromptVersion string                `json:"prompt_version"`
	Usage         *GatewayUsage         `json:"usage,omitempty"`
}

// SuggestImprovements asks the LLM service to propose improvements from one
// evaluation (EVAL-002). Same shape of contract as JudgeRun: ctx carries the
// deadline and cancellation (iron rule 7), and a 502 comes back as an error the
// caller decides about — the caller does not retry it, because a gateway that
// refused this input will refuse it again and an evaluation without suggestions is
// a complete evaluation.
func (c *Client) SuggestImprovements(
	ctx context.Context, req SuggestImprovementsRequest,
) (*SuggestImprovementsResponse, error) {
	return post[SuggestImprovementsRequest, SuggestImprovementsResponse](
		ctx, c, "/suggest-improvements", req)
}

// --- /v1/generate-skill (GEN-001, GEN-002) -----------------------------------

// GenerateSkillRequest carries the user's own words and nothing else. No
// existing Skill Version is an input: starting from one is improvement and goes
// to /suggest-improvements (ADR-046 決策 3).
type GenerateSkillRequest struct {
	TaskDescription string `json:"task_description"`
}

// GeneratedFile is one package file besides SKILL.md.
type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// GeneratedSkill is typed frontmatter plus a body, not a SKILL.md string.
//
// There is deliberately no License field, and adding one would not help: the
// schema handed to the model has no such property either, so a licence string
// cannot arrive to be dropped (ADR-046 決策 5). The value the platform stores is
// "unknown" until a person declares one.
//
// There is no Metadata field either, for a duller reason: an open-ended string
// map is `additionalProperties: {"type": "string"}`, which a strict
// `json_schema` cannot express, and no prompt ever asked the model to fill it.
// It was a spec field carried because the specification has one, producing an
// empty map on every generation.
type GeneratedSkill struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Compatibility string          `json:"compatibility,omitempty"`
	AllowedTools  string          `json:"allowed_tools,omitempty"`
	Body          string          `json:"body"`
	Files         []GeneratedFile `json:"files,omitempty"`
}

// GenerateSkillResponse separates model output (`Skill`) from what the service
// knows and the model cannot influence (`Model`, `PromptVersion`) — the two
// halves of the provenance row a generated version carries (02:GEN-002).
type GenerateSkillResponse struct {
	Skill         GeneratedSkill `json:"skill"`
	Model         string         `json:"model"`
	PromptVersion string         `json:"prompt_version"`
	Usage         *GatewayUsage  `json:"usage,omitempty"`
}

// ErrGenerateTruncated: the model hit the output ceiling. A separate failure
// class because the retry rule must not apply to it — the cap covers reasoning
// plus output, so a second call at the same cap buys the same answer
// (ADR-047 決策 2).
var ErrGenerateTruncated = errors.New("llmclient: generated skill was truncated at the token ceiling")

// truncationMarker is the WHOLE sentence apps/llm puts in the 502 detail for
// that case, not the word "truncated".
//
// The word alone was wrong, and provably so: post() builds the error from up to
// 1 KiB of the response body, and apps/llm's other 502 is
// `generate gateway error: {e}` — the LiteLLM exception text verbatim, which
// routinely quotes provider messages and the request payload. Any of those
// containing the word (a context-window notice, a provider's own "input was
// truncated", or a task description that simply uses the word and comes back in
// a 400 body) was classified as a token-ceiling truncation, and the user was
// then told to make their task smaller about a failure that had nothing to do
// with size.
//
// Still a string across a language boundary, held by a test on each side:
// test_the_truncation_sentence_is_the_one_go_matches_on asserts the
// Python half, TestTruncationComesBackAsItsOwnError the Go half — now including
// the false positive.
const truncationMarker = "generate model output was truncated at the token ceiling"

// GenerateSkill asks the LLM service to write one Skill from a task description
// (GEN-001). One call, no retry here: whether to try again is a decision made
// against the validation report, which only exists after Go has packaged the
// answer (iron rule 6, see skill/admission/generate.go).
func (c *Client) GenerateSkill(ctx context.Context, taskDescription string) (*GenerateSkillResponse, error) {
	resp, err := post[GenerateSkillRequest, GenerateSkillResponse](
		ctx, c, "/v1/generate-skill", GenerateSkillRequest{TaskDescription: taskDescription})
	if err != nil && strings.Contains(err.Error(), truncationMarker) {
		return nil, fmt.Errorf("%w: %v", ErrGenerateTruncated, err)
	}
	return resp, err
}
