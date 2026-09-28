package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestEnrichmentCostEventsCarryThePromptVersionAndEachCallsOwnUsage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrich-skill", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":"s","task_examples":[],"tags":{},"limitations":[],` +
			`"model":"enrich-model","prompt_version":"enrich-skill/v9",` +
			`"usage":{"prompt_tokens":11,"completion_tokens":12,"cost_usd":0.001,"cost_source":"gateway"}}`))
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[` + zeros(1536) + `]],"model":"embed-model","dimensions":1536,` +
			`"usage":{"prompt_tokens":21,"completion_tokens":0,"cost_usd":0.002,"cost_source":"gateway"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ledger := &fakeLedger{}
	s := &Service{LLM: ModelOrNone(&llmclient.Client{BaseURL: srv.URL}), Credit: ledger}

	if e := s.enrichPackage(context.Background(), testPackage(), mustUUIDForTest(t, "11111111-1111-1111-1111-111111111111")); e.status != enrichmentEnriched {
		t.Fatalf("enrichment status = %q, want enriched", e.status)
	}

	if len(ledger.events) != 2 {
		t.Fatalf("cost events = %d, want 2", len(ledger.events))
	}
	enrich, embed := ledger.events[0], ledger.events[1]
	if enrich.PromptVersion != "enrich-skill/v9" || enrich.PromptTokens != 11 || enrich.CompletionTokens != 12 || enrich.Estimated {
		t.Errorf("enrichment event = %+v, want prompt version enrich-skill/v9, 11/12 tokens, reported cost", enrich)
	}
	if embed.PromptTokens != 21 || embed.Estimated {
		t.Errorf("embedding event = %+v, want 21 prompt tokens and a reported cost", embed)
	}
}

func zeros(n int) string {
	b := make([]byte, 0, 2*n)
	for i := range n {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '0')
	}
	return string(b)
}
