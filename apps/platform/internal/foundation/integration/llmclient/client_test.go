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
// test_the_truncation_sentence_is_the_one_go_matches_on holds the other.
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

// /v1/embed takes an optional timeout_seconds and clamps to
// min(app.EMBED_TIMEOUT_SECONDS, it), so the platform can ask for less than the
// service's 20s ceiling. Until this existed there was no way to send it, and a
// Go-side context deadline is not a substitute: it abandons the HTTP call while
// the gateway request behind it keeps running and keeps being billed.
//
// What the two cases pin is the asymmetry, which is the part a "tidy-up" would
// flatten: search asks for less because somebody is watching, indexing does not
// because nobody is.
func TestEmbedSendsATimeoutOnlyWhenOneIsAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds float64
		want    any // the decoded JSON value of timeout_seconds, or nil for absent
	}{
		{name: "search asks for ten", seconds: 10, want: float64(10)},
		{name: "indexing keeps the service default", seconds: 0, want: nil},
		{name: "a nonsense value is not sent", seconds: -1, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_ = json.NewEncoder(w).Encode(EmbedResponse{Embeddings: [][]float32{{1}}, Dimensions: 1})
			}))
			defer srv.Close()

			c := &Client{BaseURL: srv.URL, Token: "t"}
			var err error
			if tc.seconds == 0 {
				_, err = c.Embed(context.Background(), []string{"q"})
			} else {
				_, err = c.EmbedWithin(context.Background(), []string{"q"}, tc.seconds)
			}
			if err != nil {
				t.Fatalf("embed: %v", err)
			}
			got, present := body["timeout_seconds"]
			if tc.want == nil {
				if present {
					t.Errorf("timeout_seconds = %v was sent; omitting it is what leaves the service on its own ceiling", got)
				}
				return
			}
			if !present || got != tc.want {
				t.Errorf("timeout_seconds = %v (present=%v), want %v", got, present, tc.want)
			}
		})
	}
}
