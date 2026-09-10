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

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}

	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}

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

	get(h, http.MethodOptions, "http://evil.example")
	if !reached {
		t.Fatal("unknown-origin preflight was answered here instead of falling through")
	}
}

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
