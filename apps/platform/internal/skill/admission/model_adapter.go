package ingest

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type modelOverHTTP struct{ client *llmclient.Client }

// A nil *Client inside a non-nil interface passes every `LLM != nil` guard
// and panics on the first call.
func ModelOrNone(c *llmclient.Client) Model {
	if c == nil {
		return nil
	}
	return modelOverHTTP{client: c}
}

func (a modelOverHTTP) Embed(ctx context.Context, texts []string) (*Embeddings, error) {
	resp, err := a.client.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	return &Embeddings{
		Vectors:    resp.Embeddings,
		Model:      resp.Model,
		Dimensions: resp.Dimensions,
		Usage:      usageFromWire(resp.Usage),
	}, nil
}

func (a modelOverHTTP) EnrichSkill(ctx context.Context, req EnrichRequest) (*SkillEnrichment, error) {
	resp, err := a.client.EnrichSkill(ctx, llmclient.EnrichSkillRequest{
		SkillName:      req.SkillName,
		SkillMD:        req.SkillMD,
		FileTree:       req.FileTree,
		Language:       req.Language,
		TimeoutSeconds: req.Within.Seconds(),
	})
	if err != nil {
		return nil, err
	}
	out := &SkillEnrichment{
		Summary:       resp.Summary,
		Limitations:   resp.Limitations,
		Model:         resp.Model,
		PromptVersion: resp.PromptVersion,
		Usage:         usageFromWire(resp.Usage),
		Tags: SkillTags{
			Inputs: resp.Tags.Inputs, Outputs: resp.Tags.Outputs,
			Tools: resp.Tags.Tools, Dependencies: resp.Tags.Dependencies,
		},
	}
	for _, e := range resp.TaskExamples {
		out.TaskExamples = append(out.TaskExamples, TaskExample{ZhHant: e.ZhHant, En: e.En})
	}
	for _, c := range resp.Checks {
		out.Checks = append(out.Checks, EnrichCheck{
			Rule: c.Rule, Field: c.Field, Token: c.Token, Severity: c.Severity,
		})
	}
	return out, nil
}

func (a modelOverHTTP) GenerateSkill(ctx context.Context, req GenerateRequest) (*GeneratedDraft, error) {
	wire := llmclient.GenerateSkillRequest{
		TaskDescription: req.TaskDescription,
		TimeoutSeconds:  req.Within.Seconds(),
	}
	for _, r := range req.References {
		wire.References = append(wire.References, llmclient.GenerateReference{Name: r.Name, SkillMD: r.SkillMD})
	}
	if req.Diagram != nil {
		wire.Diagram = &llmclient.GenerateDiagram{
			MediaType: req.Diagram.MediaType,
			Data:      base64.StdEncoding.EncodeToString(req.Diagram.Data),
		}
	}
	resp, err := a.client.GenerateSkill(ctx, wire)
	if err != nil {
		if errors.Is(err, llmclient.ErrGenerateTruncated) {
			return nil, errors.Join(ErrGenerationTruncated, err)
		}
		return nil, err
	}
	return &GeneratedDraft{
		Skill:         skillFromWire(resp.Skill),
		Model:         resp.Model,
		PromptVersion: resp.PromptVersion,
		Usage:         usageFromWire(resp.Usage),
	}, nil
}

func skillFromWire(g llmclient.GeneratedSkill) GeneratedSkill {
	out := GeneratedSkill{
		Name:          g.Name,
		Description:   g.Description,
		Compatibility: g.Compatibility,
		AllowedTools:  g.AllowedTools,
		Body:          g.Body,
	}
	for _, f := range g.Files {
		out.Files = append(out.Files, GeneratedFile{Path: f.Path, Content: f.Content})
	}
	return out
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
