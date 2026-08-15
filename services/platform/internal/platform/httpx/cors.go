package httpx

import "net/http"

// DevCORS allows exactly one extra origin to call this API with credentials.
// It exists for one situation and is off unless an origin is configured.
//
// Why this and not a Vite dev proxy. In production (ADR-018 E1, single node)
// the SPA and the API are served from the same origin, so there is no CORS
// problem to solve there — this must stay a local-development affordance and
// never become a deployment requirement. Locally the SPA is on :5173 and the
// API on :8080, and the usual fix is to proxy a list of path prefixes through
// the dev server. That does not work here: the API owns /skills/{id}/... while
// the SPA's own router owns the page URL /skills/$skillId. A proxy rule for
// /skills swallows the SPA's deep links, and one that does not proxy /skills
// misses the API. Any prefix list is therefore either wrong for the browser or
// wrong for fetch(), and would have to be re-derived every time a route is
// added. Routes are not the frontend's business, so the boundary moves here.
//
// Cookies survive the split without SameSite changes: SameSite is a *site*
// rule and ports are not part of a site, so localhost:5173 and localhost:8080
// are same-site and the existing Lax session cookie is sent. Only the CORS
// headers were missing.
//
// The allowed origin is echoed only on an exact match, never reflected from the
// request, and Vary: Origin keeps a cache from serving one origin's response to
// another.
func DevCORS(next http.Handler, origin string) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
