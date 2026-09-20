package eval

import (
	"context"
	"time"
)

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

type JudgedSkill struct {
	Name    string
	Summary string
}

type JudgeCriterion struct {
	ID              string
	Text            string
	EvidenceExcerpt string
}

type JudgeArtifact struct {
	Path        string
	SizeBytes   int64
	ContentType string
	TextExcerpt string
}

type TraceDigestEntry struct {
	TraceEventID string
	OccurredAt   string
	Type         string
	Excerpt      string
}

type TraceDigest struct {
	Complete bool
	Entries  []TraceDigestEntry
}

type JudgeRubricItem struct {
	ID               string
	Text             string
	Weight           *float64
	EvidenceRequired bool
}

type JudgeRubric struct {
	Items []JudgeRubricItem
}

type JudgeRequest struct {
	RunID        string
	EvaluationID string
	Skill        *JudgedSkill
	UserPrompt   string
	Criteria     []JudgeCriterion
	Rubric       *JudgeRubric
	FinalOutput  string
	Artifacts    []JudgeArtifact
	TraceDigest  TraceDigest
	Truncation   []string

	Within time.Duration
}

type Citation struct {
	Kind         string
	TraceEventID *string
	ArtifactPath *string
	Quote        string
}

type CriterionVerdict struct {
	CriterionID string
	Result      string
	Reason      string
	Citations   []Citation
}

type Judgement struct {
	Criteria      []CriterionVerdict
	Overall       string
	Summary       string
	Model         string
	PromptVersion string
	Usage         *ModelUsage
}

type Judge interface {
	JudgeRun(ctx context.Context, req JudgeRequest) (*Judgement, error)
}

type TargetFile struct {
	Path    string
	Content string
}

type ImprovementRequest struct {
	EvaluationID     string
	EvaluationDigest string
	FileTree         []string
	TargetFiles      []TargetFile

	Within time.Duration
}

type ImprovementProposal struct {
	Category        string
	Problem         string
	Evidence        string
	TargetPath      string
	ProposedContent string
	ExpectedImpact  string
}

type Improvements struct {
	Proposals     []ImprovementProposal
	Model         string
	PromptVersion string
	Usage         *ModelUsage
}

type Suggester interface {
	SuggestImprovements(ctx context.Context, req ImprovementRequest) (*Improvements, error)
}
