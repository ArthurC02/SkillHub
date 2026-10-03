package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheLoginStateCookieExpiresAndIsHiddenFromScripts(t *testing.T) {
	h := &Handler{Service: &Service{OAuth: stubGitHub(t, "x@x.dev")}}
	w := httptest.NewRecorder()
	h.startLogin(w, httptest.NewRequest(http.MethodGet, "/auth/github/login", nil))

	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookie {
			if c.MaxAge <= 0 {
				t.Errorf("state cookie MaxAge = %d, want a positive lifetime so an abandoned login stops being accepted", c.MaxAge)
			}
			if !c.HttpOnly {
				t.Error("state cookie is readable by page scripts")
			}
			return
		}
	}
	t.Fatalf("no %s cookie was set; status %d", stateCookie, w.Code)
}

func TestTheDefaultGitHubClientGivesUpEventually(t *testing.T) {
	if got := (&GitHubOAuth{}).client().Timeout; got <= 0 {
		t.Fatalf("default GitHub client timeout = %v, want a bounded wait", got)
	}
}
