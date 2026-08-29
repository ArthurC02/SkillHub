package apiserver_test

// The query-time embed asks apps/llm to stop sooner than its own ceiling.
//
// /v1/embed clamps to min(app.EMBED_TIMEOUT_SECONDS, timeout_seconds), so the
// number the platform sends can only ever shorten the wait. Until llmclient grew
// a way to send it, search had nothing but a Go context deadline — and a Go
// deadline reaches nothing: it abandons the HTTP call while the gateway request
// behind it keeps running and keeps being billed on the deployment-wide
// LITELLM_API_KEY.
//
// The 25 second budget in discovery stays as it is, over the service's ceiling
// on purpose (its `budget-over:` marker says so): it is the backstop for
// "apps/llm never answered at all", which is a different failure from "the
// gateway is slow" and needs a longer number, not a shorter one.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// recordingEmbedLLM is stubLLM's /embed half with the request body kept.
func recordingEmbedLLM(t *testing.T) (baseURL string, timeoutSent func() (any, bool)) {
	t.Helper()
	var (
		mu     sync.Mutex
		body   map[string]any
		called bool
	)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		body, called = decoded, true
		mu.Unlock()
		writeJSON(w, map[string]any{
			"embeddings": [][]float32{unitVector(7)},
			"model":      "text-embedding-3-small",
			"dimensions": embedDims,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL, func() (any, bool) {
		mu.Lock()
		defer mu.Unlock()
		if !called {
			t.Fatal("the search never called /embed, so this test proves nothing about what it sends")
		}
		v, present := body["timeout_seconds"]
		return v, present
	}
}

func TestTheQueryEmbedAsksTheServiceForTenSeconds(t *testing.T) {
	pool := requireDB(t)
	llmURL, timeoutSent := recordingEmbedLLM(t)
	a := newAPIWithLLM(t, pool, llmURL)

	// The public catalogue search, which is the hybrid path: it is what runs the
	// vector leg, and it is the one with a person waiting on the answer.
	resp, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape("整理試算表"))
	if err != nil {
		t.Fatalf("GET /api/skills/search: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search answered %d", resp.StatusCode)
	}

	got, present := timeoutSent()
	if !present {
		t.Fatal("the query embed sent no timeout_seconds; the service stays on its own 20s ceiling and " +
			"the only thing bounding the wait is a Go deadline, which does not stop the billed call")
	}
	if got != float64(10) {
		t.Errorf("timeout_seconds = %v, want 10", got)
	}
}
