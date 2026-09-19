package creation

import (
	"context"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type stepOverHTTP struct{ client *llmclient.Client }

// A nil *Client inside a non-nil interface passes every `LLM == nil` guard
// and panics on the first call.
func ModelOrNone(c *llmclient.Client) StepModel {
	if c == nil {
		return nil
	}
	return stepOverHTTP{client: c}
}

func (a stepOverHTTP) CreationStep(ctx context.Context, req StepRequest) (*StepResult, error) {
	resp, err := a.client.CreationStep(ctx, llmclient.CreationStepRequest{
		SessionID:            req.SessionID,
		Revision:             req.Revision,
		Messages:             wireMessages(req.Messages),
		Brief:                req.Brief,
		AcceptanceCriteria:   req.AcceptanceCriteria,
		SampleInput:          req.SampleInput,
		BriefConfirmed:       req.BriefConfirmed,
		DiagramUnderstanding: req.DiagramUnderstanding,
		DiagramConfirmed:     req.DiagramConfirmed,
		Diagram:              wireDiagram(req.Diagram),
		References:           wireReferences(req.References),
		Draft:                wireSkill(req.Draft),
		DraftValidation:      wireValidation(req.DraftValidation),
		AllowedTools:         req.AllowedTools,
		TimeoutSeconds:       req.TimeoutSeconds,
		MaxOutputTokens:      req.MaxOutputTokens,
		GatewayKey:           req.GatewayKey,
	})
	if err != nil {
		return nil, err
	}
	return &StepResult{
		Outcome:              resp.Outcome,
		Message:              resp.Message,
		Brief:                resp.Brief,
		AcceptanceCriteria:   resp.AcceptanceCriteria,
		SampleInput:          resp.SampleInput,
		DiagramUnderstanding: resp.DiagramUnderstanding,
		Reason:               resp.Reason,
		ToolIntent:           intentFromWire(resp.ToolIntent),
		Draft:                skillFromWire(resp.Draft),
		Model:                resp.Model,
		PromptVersion:        resp.PromptVersion,
		Usage:                usageFromWire(resp.Usage),
	}, nil
}

func wireMessages(messages []Message) []llmclient.CreationMessage {
	out := make([]llmclient.CreationMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, llmclient.CreationMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

func wireDiagram(d *Diagram) *llmclient.GenerateDiagram {
	if d == nil {
		return nil
	}
	return &llmclient.GenerateDiagram{MediaType: d.MediaType, Data: d.Data}
}

func wireReferences(refs []ReferenceSkill) []llmclient.GenerateReference {
	out := make([]llmclient.GenerateReference, 0, len(refs))
	for _, r := range refs {
		out = append(out, llmclient.GenerateReference{Name: r.Name, SkillMD: r.SkillMD})
	}
	return out
}

func wireValidation(v *DraftValidation) *llmclient.CreationDraftValidation {
	if v == nil {
		return nil
	}
	return &llmclient.CreationDraftValidation{
		ContentHash: v.ContentHash, Blocked: v.Blocked, Report: v.Report,
	}
}

func wireSkill(g *GeneratedSkill) *llmclient.GeneratedSkill {
	if g == nil {
		return nil
	}
	out := llmclient.GeneratedSkill{
		Name: g.Name, Description: g.Description, Compatibility: g.Compatibility,
		AllowedTools: g.AllowedTools, Body: g.Body,
	}
	for _, f := range g.Files {
		out.Files = append(out.Files, llmclient.GeneratedFile{Path: f.Path, Content: f.Content})
	}
	return &out
}

func skillFromWire(g *llmclient.GeneratedSkill) *GeneratedSkill {
	if g == nil {
		return nil
	}
	out := GeneratedSkill{
		Name: g.Name, Description: g.Description, Compatibility: g.Compatibility,
		AllowedTools: g.AllowedTools, Body: g.Body,
	}
	for _, f := range g.Files {
		out.Files = append(out.Files, GeneratedFile{Path: f.Path, Content: f.Content})
	}
	return &out
}

func intentFromWire(i *llmclient.CreationToolIntent) *ToolIntent {
	if i == nil {
		return nil
	}
	return &ToolIntent{Kind: i.Kind, Query: i.Query, Queries: i.Queries}
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
