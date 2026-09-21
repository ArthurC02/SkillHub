package wiring

import "testing"

func TestLLMClientHasAnExplicitHTTPClientWithoutACompetingTimeout(t *testing.T) {
	client := LLMClient("http://llm.test", "token")
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 0 {
		t.Fatalf("HTTPClient = %#v, want an explicit client whose deadline is controlled by the caller context", client.HTTPClient)
	}
}
