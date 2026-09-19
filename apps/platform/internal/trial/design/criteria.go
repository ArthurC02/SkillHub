package testlab

import "context"

type DatasetField struct {
	Name         string
	InferredType string
}

type DatasetOutline struct {
	FileName    string
	ContentType string
	Fields      []DatasetField
}

type CriteriaRequest struct {
	SkillName    string
	SkillSummary string
	UserPrompt   string
	Datasets     []DatasetOutline
}

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

type CriteriaProposal struct {
	Texts []string
	Usage *ModelUsage
}

type CriteriaSuggester interface {
	SuggestCriteria(ctx context.Context, req CriteriaRequest) (*CriteriaProposal, error)
}
