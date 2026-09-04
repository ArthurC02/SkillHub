package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedReadCloser struct {
	reader *strings.Reader
	eof    bool
	closed bool
}

func (b *trackedReadCloser) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF {
		b.eof = true
	}
	return n, err
}

func (b *trackedReadCloser) Close() error {
	b.closed = true
	return nil
}

func TestClientRejectsOversizedAndTrailingResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"past byte limit", `{}` + strings.Repeat(" ", MaxResponseBytes-1)},
		{"second JSON value", `{} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL}
			if _, err := c.Embed(context.Background(), []string{"x"}); err == nil {
				t.Fatal("client accepted a response outside its bounded JSON contract")
			}
		})
	}
}

func TestClientAcceptsAValidResponseAtTheExactByteLimit(t *testing.T) {
	prefix, suffix := `{"embeddings":[],"dimensions":0,"padding":"`, `"}`
	body := prefix + strings.Repeat("x", MaxResponseBytes-len(prefix)-len(suffix)) + suffix
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	if _, err := (&Client{BaseURL: srv.URL}).Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("exactly %d valid bytes were rejected: %v", MaxResponseBytes, err)
	}
}

func TestClientDrainsAndClosesOrdinaryErrorResponses(t *testing.T) {
	body := &trackedReadCloser{reader: strings.NewReader(strings.Repeat("x", 4096))}
	client := &http.Client{Transport: testRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: body}, nil
	})}
	c := &Client{BaseURL: "https://llm.test", HTTPClient: client}
	if _, err := c.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("error response was accepted")
	}
	if !body.eof || !body.closed {
		t.Fatalf("error body was not reusable: eof=%v closed=%v", body.eof, body.closed)
	}
}

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

			_, err := (&Client{BaseURL: srv.URL}).GenerateSkill(context.Background(),
				GenerateSkillRequest{TaskDescription: "任何任務"})
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

// A field the service sends and this client drops is the defect this repository
// has now paid for three times: `agent_output` on a run result, `run_lifecycle`
// and `llm_service` in the trace schema — each declared in a contract, each read
// by nothing, each discovered by somebody wondering why the system had nothing
// to say. `checks` is decoded, so this holds that it stays decoded.
//
// The findings are the enrichment service's own deterministic disagreements with
// the document it was given (05 R-34): a runtime the source needs that the
// limitations never mention, an appraisal the source never made, CJK inside an
// English example sentence.
func TestEnrichSkillDecodesTheServicesOwnFindings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"summary": "s",
			"task_examples": [],
			"tags": {"inputs": [], "outputs": [], "tools": [], "dependencies": []},
			"limitations": [],
			"model": "m",
			"prompt_version": "enrich-skill/v6",
			"checks": [
				{"rule": "runtime_not_in_limitations", "field": "limitations", "token": "python", "severity": "warning"},
				{"rule": "non_english_in_en_example", "field": "task_examples[0].en"}
			]
		}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "t"}
	resp, err := c.EnrichSkill(context.Background(), EnrichSkillRequest{SkillName: "x", SkillMD: "y"})
	if err != nil {
		t.Fatalf("EnrichSkill: %v", err)
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("Checks = %+v, want the two the service sent", resp.Checks)
	}
	if resp.Checks[0].Rule != "runtime_not_in_limitations" || resp.Checks[0].Token != "python" {
		t.Errorf("first finding = %+v, want the runtime rule naming python", resp.Checks[0])
	}
	// Severity is optional on the wire; a finding without one is still a finding.
	if resp.Checks[1].Field != "task_examples[0].en" {
		t.Errorf("second finding = %+v, want the English-example rule with its field path", resp.Checks[1])
	}
}

// An enrichment that passes every deterministic rule sends no findings, and that
// must stay distinguishable from a build that never checked. Empty, not absent,
// is what a passing check looks like.
func TestEnrichSkillWithoutFindingsIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"summary":"s","task_examples":[],"tags":{"inputs":[],"outputs":[],"tools":[],"dependencies":[]},"limitations":[],"model":"m","prompt_version":"v"}`))
	}))
	defer srv.Close()

	resp, err := (&Client{BaseURL: srv.URL, Token: "t"}).
		EnrichSkill(context.Background(), EnrichSkillRequest{SkillName: "x", SkillMD: "y"})
	if err != nil {
		t.Fatalf("EnrichSkill: %v", err)
	}
	if len(resp.Checks) != 0 {
		t.Errorf("Checks = %+v, want none", resp.Checks)
	}
}
