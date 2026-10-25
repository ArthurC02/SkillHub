package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type ModelUsage struct {
	PromptTokens     int64
	CompletionTokens int64

	CostUSD      *float64
	CostReported bool
	Reused       bool
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
	Inputs  []string
	Outputs []string
}

type MatchReason struct {
	SkillID string
	Reason  string
	Fits    bool
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

const (
	candidateTagsMaxItems = 8   // one-number: skillCandidateTagsMaxItems
	candidateTagRunes     = 200 // one-number: skillCandidateTagRunes
)

func candidateCapabilities(rawTags []byte) (inputs, outputs []string) {
	var tags SkillTags
	if len(rawTags) == 0 || json.Unmarshal(rawTags, &tags) != nil {
		return nil, nil
	}
	return candidateTags(tags.Inputs), candidateTags(tags.Outputs)
}

func candidateTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if runes := []rune(tag); len(runes) > candidateTagRunes {
			tag = string(runes[:candidateTagRunes])
		}
		out = append(out, tag)
		if len(out) == candidateTagsMaxItems {
			break
		}
	}
	return out
}
