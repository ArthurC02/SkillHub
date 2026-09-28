package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTheLoginStateCookieExpiresAfterTenMinutes(t *testing.T) {
	h := &Handler{Service: &Service{OAuth: stubGitHub(t, "x@x.dev")}}
	w := httptest.NewRecorder()
	h.startLogin(w, httptest.NewRequest(http.MethodGet, "/auth/github/login", nil))

	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookie {
			if c.MaxAge != 600 {
				t.Fatalf("state cookie MaxAge = %d, want 600", c.MaxAge)
			}
			return
		}
	}
	t.Fatalf("no %s cookie was set; status %d", stateCookie, w.Code)
}

func TestTheDefaultGitHubClientGivesUpAfterFifteenSeconds(t *testing.T) {
	if got := (&GitHubOAuth{}).client().Timeout; got != 15*time.Second {
		t.Fatalf("default GitHub client timeout = %v, want 15s", got)
	}
}
