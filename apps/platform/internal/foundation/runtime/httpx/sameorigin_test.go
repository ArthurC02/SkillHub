package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func passthrough() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

// The rule, one row per way a request can arrive. What this has to get right is
// not "refuse cross-site" — that is one line — but the three callers that must
// keep working: the sandbox pushing trace events with no browser headers at all,
// a top-level navigation (Sec-Fetch-Site: none), and every GET.
func TestSameOriginWritesRefusesOnlyCrossSiteWrites(t *testing.T) {
	const app = "https://hub.example.test"
	for _, tc := range []struct {
		name      string
		method    string
		fetchSite string
		origin    string
		want      int
	}{
		{name: "same-origin POST", method: http.MethodPost, fetchSite: "same-origin", want: http.StatusOK},
		{name: "typed URL or bookmark", method: http.MethodPost, fetchSite: "none", want: http.StatusOK},
		{name: "cross-site POST", method: http.MethodPost, fetchSite: "cross-site", want: http.StatusForbidden},
		{name: "a sibling subdomain is not this app", method: http.MethodPost, fetchSite: "same-site", want: http.StatusForbidden},
		{name: "older browser, matching Origin", method: http.MethodPost, origin: app, want: http.StatusOK},
		{name: "older browser, matching Origin with a path", method: http.MethodPost, origin: app + "/skills", want: http.StatusOK},
		{name: "older browser, foreign Origin", method: http.MethodPost, origin: "https://evil.example", want: http.StatusForbidden},
		{name: "the sandbox posting a trace, no browser headers", method: http.MethodPost, want: http.StatusOK},
		{name: "DELETE is a write too", method: http.MethodDelete, fetchSite: "cross-site", want: http.StatusForbidden},
		{name: "PATCH is a write too", method: http.MethodPatch, origin: "https://evil.example", want: http.StatusForbidden},
		{name: "a cross-site GET still passes", method: http.MethodGet, fetchSite: "cross-site", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/skills", nil)
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			SameOriginWrites(passthrough(), app).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("%s %s -> %d, want %d", tc.method, tc.name, rec.Code, tc.want)
			}
		})
	}
}

// Unset APP_URL is the shipped default and must change nothing, the same
// acceptance shape 02:PORT-005 asks of clean mode: a middleware that only turns
// on when configured and one that is always on look identical in a screenshot.
func TestSameOriginWritesIsOffWithoutAnAppURL(t *testing.T) {
	for _, appURL := range []string{"", "   ", "not a url", "://broken"} {
		req := httptest.NewRequest(http.MethodPost, "/skills", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		SameOriginWrites(passthrough(), appURL).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("APP_URL=%q refused a request; an absent or unparseable origin must disable the check, not enable a broken one", appURL)
		}
	}
}
