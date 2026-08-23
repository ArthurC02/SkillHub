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

// The Go half of a string held across a language boundary. apps/llm's
// test_truncation_is_a_different_failure_from_malformed_output holds the other.
//
// Both failures come back as 502, and the round-A truncation emitted an EMPTY
// string after spending its whole budget reasoning - so without this the two are
// indistinguishable and the generation path retries a call that cannot answer
// differently (ADR-047 決策 2).
func TestTruncationComesBackAsItsOwnError(t *testing.T) {
	for _, tc := range []struct {
		name, detail string
		want         bool
	}{
		{"truncated", "generate model output was truncated at the token ceiling", true},
		{"malformed", "generate model returned malformed output", false},
		// The false positive the bare word "truncated" used to produce. This is
		// the shape apps/llm's OTHER 502 has — the gateway exception verbatim —
		// and the user was told to shorten a task that was never too long.
		{"gateway error quoting the word", "generate gateway error: provider said the input was truncated upstream", false},
		// And the same word arriving from the user's own text, echoed back.
		{"user text quoting the word", "generate gateway error: 400 on prompt \"my logs are truncated\"", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"detail": tc.detail})
			}))
			defer srv.Close()

			_, err := (&Client{BaseURL: srv.URL}).GenerateSkill(context.Background(), "任何任務")
			if err == nil {
				t.Fatal("a 502 came back as success")
			}
			if got := errors.Is(err, ErrGenerateTruncated); got != tc.want {
				t.Errorf("errors.Is(err, ErrGenerateTruncated) = %v, want %v (err: %v)", got, tc.want, err)
			}
		})
	}
}
