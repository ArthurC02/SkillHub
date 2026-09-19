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

type CriteriaSuggester interface {
	SuggestCriteria(ctx context.Context, req CriteriaRequest) (proposed []string, err error)
}
