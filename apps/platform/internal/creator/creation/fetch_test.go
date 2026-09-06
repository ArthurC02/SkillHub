package creation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The URL rule runs before the person is asked: only public http(s) hosts.
func TestValidateFetchURLRefusesWhatMustNeverBeAsked(t *testing.T) {
	for _, raw := range []string{"ftp://example.com/x", "file:///etc/passwd", "http://user:pw@example.com/", "http://127.0.0.1/", "http://10.1.2.3/", "http://[::1]/", "http://169.254.169.254/latest/meta-data", "example.com/no-scheme", ""} {
		if _, err := validateFetchURL(raw); err == nil {
			t.Fatalf("%q was accepted", raw)
		}
	}
	got, err := validateFetchURL("  https://Example.com/a?b=1#frag ")
	if err != nil || got != "https://Example.com/a?b=1" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

// A page comes back as prose with its markup and scripts gone; the record
// carries where it came from and how big it was.
func TestFetcherReadsTextAndReportsBlocksWithoutRetry(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><head><style>p{}</style><script>alert(1)</script></head><body><h1>退貨規則</h1><p>七天內可退。</p></body></html>"))
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/flaky":
			w.WriteHeader(http.StatusBadGateway)
		case "/binary":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF"))
		case "/inside":
			http.Redirect(w, r, "http://10.0.0.1/secret", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	f := NewFetcher(true)
	ctx := context.Background()

	rec, text := f.Fetch(ctx, srv.URL+"/page")
	if rec.Status != "ok" || text != "退貨規則 七天內可退。" || rec.SHA256 == "" || rec.Bytes == 0 {
		t.Fatalf("page: %+v %q", rec, text)
	}
	if rec, _ := f.Fetch(ctx, srv.URL+"/forbidden"); rec.Status != "blocked" || hits["/forbidden"] != 1 {
		t.Fatalf("a refusal is reported once, not retried: %+v hits=%d", rec, hits["/forbidden"])
	}
	if rec, _ := f.Fetch(ctx, srv.URL+"/flaky"); rec.Status != "network_error" || hits["/flaky"] != 2 {
		t.Fatalf("a network-level failure gets one retry: %+v hits=%d", rec, hits["/flaky"])
	}
	if rec, _ := f.Fetch(ctx, srv.URL+"/binary"); rec.Status != "unsupported" {
		t.Fatalf("binary: %+v", rec)
	}
	if rec, _ := f.Fetch(ctx, srv.URL+"/inside"); rec.Status != "blocked" {
		t.Fatalf("a redirect into a private address is blocked: %+v", rec)
	}
	if rec, _ := f.Fetch(ctx, srv.URL+"/missing"); rec.Status != "not_found" {
		t.Fatalf("missing: %+v", rec)
	}
	// Production wiring refuses loopback outright.
	if rec, _ := NewFetcher(false).Fetch(ctx, srv.URL+"/page"); rec.Status != "blocked" {
		t.Fatalf("loopback must be blocked outside tests: %+v", rec)
	}
}

// The observation is JSON the model reads; the sentence is Go's and the page
// text is inside the JSON, never concatenated into prose.
func TestFetchObservationIsJSONWithGoSentence(t *testing.T) {
	obs := fetchObservation(Fetch{URL: "https://x.test/a", Status: "blocked"}, "")
	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(obs), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, obs)
	}
	if !strings.Contains(parsed["fetch"]["note"].(string), "不會重試") {
		t.Fatalf("the owner's rule is not in the sentence: %s", obs)
	}
}
