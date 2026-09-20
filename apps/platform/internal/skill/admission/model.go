package ingest

import (
	"context"
	"errors"
	"time"
)

var ErrGenerationTruncated = errors.New("ingest: the generated skill stopped at the model's token ceiling")

type ModelUsage struct {
	PromptTokens     int64
	CompletionTokens int64

	CostUSD      *float64
	CostReported bool
}

func (u *ModelUsage) ReportedCostUSD() *float64 {
	if u == nil || !u.CostReported {
		return nil
	}
	return u.CostUSD
}

type Embeddings struct {
	Vectors    [][]float32
	Model      string
	Dimensions int
	Usage      *ModelUsage
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

type EnrichCheck struct {
	Rule     string
	Field    string
	Token    string
	Severity string
}

type EnrichRequest struct {
	SkillName string
	SkillMD   string
	FileTree  []string
	Language  string

	Within time.Duration
}

type SkillEnrichment struct {
	Summary       string
	TaskExamples  []TaskExample
	Tags          SkillTags
	Limitations   []string
	Model         string
	PromptVersion string
	Checks        []EnrichCheck
	Usage         *ModelUsage
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

type ReferenceSkill struct {
	Name    string
	SkillMD string
}

type GenerateRequest struct {
	TaskDescription string
	Diagram         *GenerateDiagram
	References      []ReferenceSkill

	Within time.Duration
}

type GeneratedDraft struct {
	Skill         GeneratedSkill
	Model         string
	PromptVersion string
	Usage         *ModelUsage
}

type Model interface {
	Embed(ctx context.Context, texts []string) (*Embeddings, error)
	EnrichSkill(ctx context.Context, req EnrichRequest) (*SkillEnrichment, error)
	GenerateSkill(ctx context.Context, req GenerateRequest) (*GeneratedDraft, error)
}
