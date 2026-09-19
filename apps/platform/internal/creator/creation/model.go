package creation

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

type ModelUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`

	CostUSD      *float64 `json:"cost_usd"`
	CostReported bool     `json:"cost_reported"`
}

func (u *ModelUsage) ReportedCostUSD() *float64 {
	if u == nil || !u.CostReported {
		return nil
	}
	return u.CostUSD
}

type Diagram struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ReferenceSkill struct {
	Name    string
	SkillMD string
}

type DraftValidation struct {
	ContentHash string
	Blocked     bool
	Report      string
}

type ToolIntent struct {
	Kind    string
	Query   string
	Queries []string
}

type StepRequest struct {
	SessionID            string
	Revision             int64
	Messages             []Message
	Brief                string
	AcceptanceCriteria   []string
	SampleInput          string
	BriefConfirmed       bool
	DiagramUnderstanding string
	DiagramConfirmed     bool
	Diagram              *Diagram
	References           []ReferenceSkill
	Draft                *GeneratedSkill
	DraftValidation      *DraftValidation
	AllowedTools         []string
	TimeoutSeconds       int
	MaxOutputTokens      int
	GatewayKey           string
}

type StepResult struct {
	Outcome              string
	Message              string
	Brief                string
	AcceptanceCriteria   []string
	SampleInput          string
	DiagramUnderstanding string

	Reason        string
	ToolIntent    *ToolIntent
	Draft         *GeneratedSkill
	Model         string
	PromptVersion string
	Usage         *ModelUsage
}

type StepModel interface {
	CreationStep(ctx context.Context, req StepRequest) (*StepResult, error)
}
