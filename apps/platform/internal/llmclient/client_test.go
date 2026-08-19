package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientAuthenticatesToTheInternalService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer service-secret" {
			t.Fatalf("Authorization = %q, want service bearer", got)
		}
		_ = json.NewEncoder(w).Encode(EmbedResponse{Embeddings: [][]float32{{1}}, Dimensions: 1})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "service-secret"}
	if _, err := c.Embed(context.Background(), []string{"anything"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

// The client must impose no deadline of its own: the caller's ctx is the only
// one. A second, shorter client-side timeout is invisible to the caller, fires
// before the budget the caller set, and does not stop the upstream call - so
// the work is billed and thrown away.
func TestClientDeadlineIsTheCallersContext(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked // never answers; only ctx can end this call
	}))
	defer srv.Close()
	defer close(blocked)

	c := &Client{BaseURL: srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Embed(ctx, []string{"anything"})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	// Generous, but far below any fixed client timeout worth having: the point
	// is that the caller's 100ms ended the call, not a hidden longer one.
	if elapsed > 5*time.Second {
		t.Errorf("call took %s; the caller's deadline did not end it", elapsed)
	}
}
