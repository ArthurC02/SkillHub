package catalog

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

func (u *ModelUsage) reportedCostUSD() *float64 {
	if u == nil || !u.CostReported {
		return nil
	}
	return u.CostUSD
}

type Embeddings struct {
	Vectors [][]float32
	Model   string
	Usage   *ModelUsage
}

type SkillCandidate struct {
	SkillID string
	Name    string
	Summary string
}

type MatchReason struct {
	SkillID string
	Reason  string
}

type MatchReasons struct {
	Reasons []MatchReason
	Model   string
	Usage   *ModelUsage
}

type Model interface {
	Embed(ctx context.Context, texts []string, within time.Duration) (*Embeddings, error)
	MatchReasons(ctx context.Context, query string, candidates []SkillCandidate, within time.Duration) (*MatchReasons, error)
}

type SkillTags struct {
	Inputs       []string `json:"inputs"`
	Outputs      []string `json:"outputs"`
	Tools        []string `json:"tools"`
	Dependencies []string `json:"dependencies"`
}
