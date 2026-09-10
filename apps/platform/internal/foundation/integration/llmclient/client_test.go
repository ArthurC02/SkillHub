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

func TestClientDeadlineIsTheCallersContext(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
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

	if elapsed > 5*time.Second {
		t.Errorf("call took %s; the caller's deadline did not end it", elapsed)
	}
}

func TestTruncationComesBackAsItsOwnError(t *testing.T) {
	for _, tc := range []struct {
		name, detail string
		want         bool
	}{
		{"truncated", "generate model output was truncated at the token ceiling", true},
		{"malformed", "generate model returned malformed output", false},

		{"gateway error quoting the word", "generate gateway error: provider said the input was truncated upstream", false},

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

func TestEmbedSendsATimeoutOnlyWhenOneIsAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds float64
		want    any
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

	if resp.Checks[1].Field != "task_examples[0].en" {
		t.Errorf("second finding = %+v, want the English-example rule with its field path", resp.Checks[1])
	}
}

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
