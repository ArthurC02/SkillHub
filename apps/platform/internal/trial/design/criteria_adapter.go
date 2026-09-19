package testlab

import (
	"context"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type criteriaOverHTTP struct{ client *llmclient.Client }

// A nil *Client inside a non-nil interface passes every `LLM == nil` guard
// and panics on the first call.
func ModelOrNone(c *llmclient.Client) CriteriaSuggester {
	if c == nil {
		return nil
	}
	return criteriaOverHTTP{client: c}
}

func (a criteriaOverHTTP) SuggestCriteria(ctx context.Context, req CriteriaRequest) ([]string, error) {
	resp, err := a.client.SuggestCriteria(ctx, llmclient.SuggestCriteriaRequest{
		SkillName:    req.SkillName,
		SkillSummary: req.SkillSummary,
		UserPrompt:   req.UserPrompt,
		Datasets:     wireDatasets(req.Datasets),
	})
	if err != nil {
		return nil, err
	}
	proposed := make([]string, 0, len(resp.Criteria))
	for _, c := range resp.Criteria {
		proposed = append(proposed, c.Text)
	}
	return proposed, nil
}

func wireDatasets(datasets []DatasetOutline) []llmclient.DatasetOutline {
	out := make([]llmclient.DatasetOutline, 0, len(datasets))
	for _, d := range datasets {
		fields := make([]llmclient.DatasetField, 0, len(d.Fields))
		for _, f := range d.Fields {
			fields = append(fields, llmclient.DatasetField{Name: f.Name, InferredType: f.InferredType})
		}
		out = append(out, llmclient.DatasetOutline{
			FileName: d.FileName, ContentType: d.ContentType, Fields: fields,
		})
	}
	return out
}
