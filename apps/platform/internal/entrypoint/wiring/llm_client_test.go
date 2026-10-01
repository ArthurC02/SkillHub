package wiring

import (
	"net/http"
	"testing"
)

func TestLLMClientHasAnExplicitHTTPClientWithoutACompetingTimeout(t *testing.T) {
	client := LLMClient("http://llm.test", "token")
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 0 {
		t.Fatalf("HTTPClient = %#v, want an explicit client whose deadline is controlled by the caller context", client.HTTPClient)
	}
}

func TestEveryWorkerCanHoldItsConnectionToTheModelServiceOpen(t *testing.T) {
	transport, ok := LLMClient("http://llm.test", "token").HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("the model service client does not use the shared internal transport")
	}
	workers := 0
	for _, queue := range Queues {
		workers += queue.MaxWorkers
	}
	if transport.MaxIdleConnsPerHost < workers {
		t.Fatalf("%d idle connections kept per host for %d workers that may call it at once; the rest reconnect every call",
			transport.MaxIdleConnsPerHost, workers)
	}
	if transport != internalTransport() {
		t.Error("the model service client built its own transport instead of sharing the internal one")
	}
}
