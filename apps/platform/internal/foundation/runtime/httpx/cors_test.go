package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func get(h http.Handler, method, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/skills/search", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const devOrigin = "http://localhost:5173"

// The whole point of the flag: an unconfigured process is a production process,
// and it must not emit a CORS header at all.
func TestDevCORSDisabledByDefault(t *testing.T) {
	w := get(DevCORS(okHandler(), ""), http.MethodGet, devOrigin)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured process allowed origin %q", got)
	}
}

func TestDevCORSAllowsOnlyTheConfiguredOrigin(t *testing.T) {
	h := DevCORS(okHandler(), devOrigin)

	w := get(h, http.MethodGet, devOrigin)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != devOrigin {
		t.Fatalf("allow-origin = %q, want %q", got, devOrigin)
	}
	// Credentials are the reason this exists: the session cookie has to ride
	// along, and a browser drops it without this header.
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}
	// Without Vary, a cache can hand one origin's allowance to another.
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}

	// Anything else gets nothing — the request header is never reflected back,
	// which with credentials enabled would allow every site on the internet.
	for _, origin := range []string{"http://evil.example", "http://localhost:5174", ""} {
		w := get(h, http.MethodGet, origin)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("origin %q was allowed as %q", origin, got)
		}
	}
}

func TestDevCORSAnswersPreflightWithoutReachingTheHandler(t *testing.T) {
	reached := false
	h := DevCORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}), devOrigin)

	w := get(h, http.MethodOptions, devOrigin)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
	if reached {
		t.Fatal("preflight was routed to the application handler")
	}
	// A preflight from an unknown origin is not short-circuited: it falls
	// through with no allowance and the browser refuses it on its own.
	get(h, http.MethodOptions, "http://evil.example")
	if !reached {
		t.Fatal("unknown-origin preflight was answered here instead of falling through")
	}
}

// The header this file did not check for three tests. Deleting the
// Allow-Methods line left every other assertion here green while the browser
// refused every PATCH and PUT the SPA sends (testcases.ts, evaluation.ts).
//
// The list is what apiserver.NewRouter registers — GET, POST, PUT, PATCH,
// DELETE — plus OPTIONS for the preflight itself. Not asserted by importing
// that package: it imports httpx, so the test binary would not link.
func TestDevCORSPreflightAllowsEveryMethodTheAPIServes(t *testing.T) {
	w := get(DevCORS(okHandler(), devOrigin), http.MethodOptions, devOrigin)
	allowed := w.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	} {
		if !strings.Contains(allowed, method) {
			t.Errorf("preflight omits %s; the browser refuses that request before it reaches a handler (allow-methods = %q)",
				method, allowed)
		}
	}
}
