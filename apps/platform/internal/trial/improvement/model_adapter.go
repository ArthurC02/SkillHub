package eval

import (
	"context"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type modelOverHTTP struct{ client *llmclient.Client }

// A nil *Client inside a non-nil interface passes every `== nil` guard and
// panics on the first call.
func JudgeOrNone(c *llmclient.Client) Judge {
	if c == nil {
		return nil
	}
	return modelOverHTTP{client: c}
}

func SuggesterOrNone(c *llmclient.Client) Suggester {
	if c == nil {
		return nil
	}
	return modelOverHTTP{client: c}
}

func (a modelOverHTTP) JudgeRun(ctx context.Context, req JudgeRequest) (*Judgement, error) {
	resp, err := a.client.JudgeRun(ctx, llmclient.JudgeRunRequest{
		RunID:        req.RunID,
		EvaluationID: req.EvaluationID,
		Skill:        wireSkill(req.Skill),
		UserPrompt:   req.UserPrompt,
		Criteria:     wireCriteria(req.Criteria),
		Rubric:       wireRubric(req.Rubric),
		FinalOutput:  req.FinalOutput,
		Artifacts:    wireArtifacts(req.Artifacts),
		TraceDigest:  wireDigest(req.TraceDigest),
		Truncation:   req.Truncation,

		TimeoutSeconds: req.Within.Seconds(),
	})
	if err != nil {
		return nil, err
	}
	return &Judgement{
		Criteria:      criteriaFromWire(resp.Verdict.CriterionResults),
		Overall:       resp.Verdict.Overall,
		Summary:       resp.Verdict.Summary,
		Model:         resp.Model,
		PromptVersion: resp.PromptVersion,
		Usage:         usageFromWire(resp.Usage),
	}, nil
}

func (a modelOverHTTP) SuggestImprovements(ctx context.Context, req ImprovementRequest) (*Improvements, error) {
	files := make([]llmclient.TargetFile, 0, len(req.TargetFiles))
	for _, f := range req.TargetFiles {
		files = append(files, llmclient.TargetFile{Path: f.Path, Content: f.Content})
	}
	resp, err := a.client.SuggestImprovements(ctx, llmclient.SuggestImprovementsRequest{
		EvaluationID:     req.EvaluationID,
		EvaluationDigest: req.EvaluationDigest,
		FileTree:         req.FileTree,
		TargetFiles:      files,
		TimeoutSeconds:   req.Within.Seconds(),
	})
	if err != nil {
		return nil, err
	}
	out := &Improvements{
		Proposals:     make([]ImprovementProposal, 0, len(resp.Suggestions)),
		Model:         resp.Model,
		PromptVersion: resp.PromptVersion,
		Usage:         usageFromWire(resp.Usage),
	}
	for _, p := range resp.Suggestions {
		out.Proposals = append(out.Proposals, ImprovementProposal{
			Category:        p.Category,
			Problem:         p.Problem,
			Evidence:        p.Evidence,
			TargetPath:      p.TargetPath,
			ProposedContent: p.ProposedContent,
			ExpectedImpact:  p.ExpectedImpact,
		})
	}
	return out, nil
}

func usageFromWire(u *llmclient.GatewayUsage) *ModelUsage {
	if u == nil {
		return nil
	}
	return &ModelUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		CostUSD:          u.CostUSD,
		CostReported:     u.ReportedCostUSD() != nil,
	}
}

func wireSkill(s *JudgedSkill) *llmclient.JudgeSkill {
	if s == nil {
		return nil
	}
	return &llmclient.JudgeSkill{Name: s.Name, Summary: s.Summary}
}

func wireCriteria(criteria []JudgeCriterion) []llmclient.JudgeCriterion {
	out := make([]llmclient.JudgeCriterion, 0, len(criteria))
	for _, c := range criteria {
		out = append(out, llmclient.JudgeCriterion{
			ID: c.ID, Text: c.Text, EvidenceExcerpt: c.EvidenceExcerpt,
		})
	}
	return out
}

func wireArtifacts(artifacts []JudgeArtifact) []llmclient.JudgeArtifact {
	out := make([]llmclient.JudgeArtifact, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, llmclient.JudgeArtifact{
			Path: a.Path, SizeBytes: a.SizeBytes,
			ContentType: a.ContentType, TextExcerpt: a.TextExcerpt,
		})
	}
	return out
}

func wireDigest(d TraceDigest) llmclient.TraceDigest {
	entries := make([]llmclient.TraceDigestEntry, 0, len(d.Entries))
	for _, e := range d.Entries {
		entries = append(entries, llmclient.TraceDigestEntry{
			TraceEventID: e.TraceEventID, OccurredAt: e.OccurredAt,
			Type: e.Type, Excerpt: e.Excerpt,
		})
	}
	return llmclient.TraceDigest{Complete: d.Complete, Entries: entries}
}

func wireRubric(r *JudgeRubric) *llmclient.Rubric {
	if r == nil {
		return nil
	}
	items := make([]llmclient.RubricItem, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, llmclient.RubricItem{
			ID: it.ID, Text: it.Text, Weight: it.Weight, EvidenceRequired: it.EvidenceRequired,
		})
	}
	return &llmclient.Rubric{Items: items}
}

func criteriaFromWire(results []llmclient.CriterionVerdict) []CriterionVerdict {
	out := make([]CriterionVerdict, 0, len(results))
	for _, cv := range results {
		v := CriterionVerdict{
			CriterionID: cv.CriterionID,
			Result:      cv.Result,
			Reason:      cv.Reason,
			Citations:   make([]Citation, 0, len(cv.EvidenceRefs)),
		}
		for _, ref := range cv.EvidenceRefs {
			v.Citations = append(v.Citations, Citation{
				Kind:         ref.Kind,
				TraceEventID: ref.TraceEventID,
				ArtifactPath: ref.ArtifactPath,
				Quote:        ref.Quote,
			})
		}
		out = append(out, v)
	}
	return out
}
