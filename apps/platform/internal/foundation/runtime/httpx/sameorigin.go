package httpx

import (
	"net/http"
	"strings"
)

// SameOriginWrites checks Sec-Fetch-Site first and falls back to Origin only
// when that header is absent; a request with neither header set passes, since
// only non-browser clients omit both.
func SameOriginWrites(next http.Handler, appURL string) http.Handler {
	want := originOf(appURL)
	if want == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:

			next.ServeHTTP(w, r)
			return
		}
		switch site := r.Header.Get("Sec-Fetch-Site"); site {
		case "same-origin", "none":
			next.ServeHTTP(w, r)
			return
		case "":

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
