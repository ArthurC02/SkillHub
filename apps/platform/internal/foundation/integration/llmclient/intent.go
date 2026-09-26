package llmclient

import "context"

type AnalyzeIntentRequest struct {
	Query          string  `json:"query"`
	TimeoutSeconds float64 `json:"timeout_seconds"`
}

type AnalyzeIntentResponse struct {
	Valid         bool               `json:"valid"`
	Intent        map[string]*string `json:"intent"`
	Keywords      []string           `json:"keywords"`
	Filters       map[string]string  `json:"filters"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"prompt_version"`
	Usage         *GatewayUsage      `json:"usage,omitempty"`
}

func (c *Client) AnalyzeIntent(ctx context.Context, query string, seconds float64) (*AnalyzeIntentResponse, error) {
	return post[AnalyzeIntentRequest, AnalyzeIntentResponse](ctx, c, "/v1/analyze-intent", AnalyzeIntentRequest{Query: query, TimeoutSeconds: seconds})
}
