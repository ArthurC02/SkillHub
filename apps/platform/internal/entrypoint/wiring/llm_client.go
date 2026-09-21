package wiring

import (
	"net/http"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func LLMClient(baseURL, token string) *llmclient.Client {
	return &llmclient.Client{BaseURL: baseURL, Token: token, HTTPClient: &http.Client{}}
}
