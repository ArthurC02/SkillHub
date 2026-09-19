package catalog

import (
	"context"
	"time"

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

func (a modelOverHTTP) Embed(ctx context.Context, texts []string, within time.Duration) (*Embeddings, error) {
	resp, err := a.client.EmbedWithin(ctx, texts, within.Seconds())
	if err != nil {
		return nil, err
	}
	return &Embeddings{Vectors: resp.Embeddings, Model: resp.Model, Usage: usageFromWire(resp.Usage)}, nil
}

func (a modelOverHTTP) MatchReasons(ctx context.Context, query string, candidates []SkillCandidate) (*MatchReasons, error) {
	wire := make([]llmclient.SkillCandidate, 0, len(candidates))
	for _, c := range candidates {
		wire = append(wire, llmclient.SkillCandidate{SkillID: c.SkillID, Name: c.Name, Summary: c.Summary})
	}
	resp, err := a.client.MatchReasons(ctx, query, wire)
	if err != nil {
		return nil, err
	}
	out := &MatchReasons{
		Reasons: make([]MatchReason, 0, len(resp.Reasons)),
		Model:   resp.Model,
		Usage:   usageFromWire(resp.Usage),
	}
	for _, r := range resp.Reasons {
		out.Reasons = append(out.Reasons, MatchReason{SkillID: r.SkillID, Reason: r.Reason})
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
