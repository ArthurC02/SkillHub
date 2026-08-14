package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func stubGitHub(t *testing.T, profileEmail string) *GitHubOAuth {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("code") != "good-code" {
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := "null"
		if profileEmail != "" {
			email = `"` + profileEmail + `"`
		}
		_, _ = w.Write([]byte(`{"id":42,"login":"arthur","name":"","email":` + email + `}`))
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"email":"old@x.dev","primary":false,"verified":true},
			{"email":"a@x.dev","primary":true,"verified":true}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &GitHubOAuth{
		ClientID: "cid", ClientSecret: "sec", RedirectURL: "http://app/cb",
		AuthBase: srv.URL, APIBase: srv.URL,
	}
}

func TestAuthURLCarriesState(t *testing.T) {
	g := stubGitHub(t, "")
	u, err := url.Parse(g.AuthURL("st4te"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != "st4te" || q.Get("client_id") != "cid" || q.Get("scope") != "user:email" {
		t.Fatalf("unexpected auth url query: %v", q)
	}
}

func TestExchange(t *testing.T) {
	g := stubGitHub(t, "")
	if _, err := g.Exchange(context.Background(), "bad-code"); err == nil {
		t.Fatal("want error for bad code")
	}
	tok, err := g.Exchange(context.Background(), "good-code")
	if err != nil || tok != "tok" {
		t.Fatalf("got %q, %v", tok, err)
	}
}

func TestFetchUserEmailFallbackAndNameDefault(t *testing.T) {
	g := stubGitHub(t, "")
	u, err := g.FetchUser(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "a@x.dev" {
		t.Fatalf("want primary verified email, got %q", u.Email)
	}
	if u.Name != "arthur" {
		t.Fatalf("want login as name fallback, got %q", u.Name)
	}
	if u.ID != 42 {
		t.Fatalf("want id 42, got %d", u.ID)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	h := &Handler{Service: &Service{OAuth: stubGitHub(t, "x@x.dev")}}

	// No state cookie at all.
	r := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=abc", nil)
	w := httptest.NewRecorder()
	h.finishLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing cookie: want 401, got %d", w.Code)
	}

	// Cookie present but different value.
	r = httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=abc", nil)
	r.AddCookie(&http.Cookie{Name: stateCookie, Value: "other"})
	w = httptest.NewRecorder()
	h.finishLogin(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched state: want 401, got %d", w.Code)
	}
}

func TestRequireSessionWithoutCookie(t *testing.T) {
	h := &Handler{Service: &Service{}}
	called := false
	next := h.RequireSession(func(http.ResponseWriter, *http.Request) { called = true })
	w := httptest.NewRecorder()
	next(w, httptest.NewRequest(http.MethodGet, "/me", nil))
	if w.Code != http.StatusUnauthorized || called {
		t.Fatalf("want 401 without cookie, got %d (called=%v)", w.Code, called)
	}
	if !strings.Contains(w.Body.String(), "not authenticated") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}
