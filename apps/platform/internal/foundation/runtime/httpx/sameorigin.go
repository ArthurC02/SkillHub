package httpx

import (
	"net/http"
	"strings"
)

// SameOriginWrites refuses a cross-site write.
//
// Until this existed the only thing standing between a signed-in user and a
// cross-site POST was the session cookie's SameSite=Lax, and that is a browser
// default rather than an assertion this platform makes: it covers the sixty-odd
// mutating routes today, and it stops covering all of them at once the day
// somebody needs an embed and changes one line to SameSite=None. There is no
// CSRF token anywhere in this codebase (a deliberate choice — a token store for
// a same-origin SPA is over-built), so this middleware is the second line, and
// it is the whole of it.
//
// The rule, in the order a browser answers it:
//
//  1. Sec-Fetch-Site says same-origin or none (a typed URL, a bookmark) — allow.
//     Cross-site and same-site both refuse; same-site is included because a
//     sibling subdomain is not this app.
//  2. No Sec-Fetch-Site, but an Origin — it must equal appURL's origin.
//  3. Neither header — allow. This is not a hole a browser can walk through:
//     every browser that can be told to make a cross-site request has sent
//     Origin on cross-origin writes for years, and Sec-Fetch-Site since 2020.
//     What arrives with neither is a non-browser client, and this API has
//     several that must keep working: the sandbox provider pushing trace events
//     (TRACE-002, authenticated by its own signed per-attempt token), curl, and
//     the E2E suite. Refusing them would be refusing the callers that were never
//     the threat — a CSRF is by definition a request a BROWSER was tricked into
//     making with somebody's cookie attached.
//
// appURL empty disables it entirely, which is the shipped default and the state
// every deployment that has not set APP_URL is in. Deliberately not "refuse
// everything when unconfigured": this is defence in depth behind SameSite=Lax,
// and a protection nobody configured must not take the platform down.
func SameOriginWrites(next http.Handler, appURL string) http.Handler {
	want := originOf(appURL)
	if want == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			// GET and HEAD pass. A state-changing GET does exist here — the
			// download route writes a record and a funnel event — and it stays
			// out on purpose: refusing GETs by Origin would break every ordinary
			// top-level navigation, and what that particular route leaks is a
			// row, not bytes (the response never reaches the attacker's page).
			next.ServeHTTP(w, r)
			return
		}
		switch site := r.Header.Get("Sec-Fetch-Site"); site {
		case "same-origin", "none":
			next.ServeHTTP(w, r)
			return
		case "":
			// Fall through to the Origin check.
		default:
			WriteError(w, http.StatusForbidden, "跨站的寫入請求已被拒絕。")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && originOf(origin) != want {
			WriteError(w, http.StatusForbidden, "跨站的寫入請求已被拒絕。")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originOf reduces a URL to scheme://host, which is what an Origin header is.
// Deliberately string surgery and not net/url: an APP_URL that does not parse
// must disable the check rather than panic at start-up, and a comparison of two
// strings is what the header is anyway.
func originOf(raw string) string {
	raw = strings.TrimSpace(raw)
	scheme, rest, found := strings.Cut(raw, "://")
	if !found || scheme == "" || rest == "" {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	if host == "" {
		return ""
	}
	return strings.ToLower(scheme) + "://" + strings.ToLower(host)
}
