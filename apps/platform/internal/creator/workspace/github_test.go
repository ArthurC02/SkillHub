package identity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type eofTrackingBody struct {
	reader *strings.Reader
	eof    bool
	closed bool
}

func (b *eofTrackingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF {
		b.eof = true
	}
	return n, err
}

func (b *eofTrackingBody) Close() error {
	b.closed = true
	return nil
}

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

func TestGitHubJSONRejectsOversizedAndTrailingResponses(t *testing.T) {
	for _, body := range []string{
		`{}` + strings.Repeat(" ", maxGitHubResponseBytes-1),
		`{} {}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		err = (&GitHubOAuth{Client: srv.Client()}).doJSON(req, &out)
		srv.Close()
		if err == nil {
			t.Fatal("GitHub client accepted a response outside its bounded JSON contract")
		}
	}
}

func TestGitHubJSONAcceptsAValidResponseAtTheExactByteLimit(t *testing.T) {
	prefix, suffix := `{"padding":"`, `"}`
	body := prefix + strings.Repeat("x", maxGitHubResponseBytes-len(prefix)-len(suffix)) + suffix
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := (&GitHubOAuth{Client: srv.Client()}).doJSON(req, &out); err != nil {
		t.Fatalf("exactly %d valid bytes were rejected: %v", maxGitHubResponseBytes, err)
	}
}

func TestGitHubJSONDrainsAndClosesOrdinaryErrorResponses(t *testing.T) {
	body := &eofTrackingBody{reader: strings.NewReader(strings.Repeat("x", 4096))}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: body}, nil
	})}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.test/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := (&GitHubOAuth{Client: client}).doJSON(req, &out); err == nil {
		t.Fatal("error response was accepted")
	}
	if !body.eof || !body.closed {
		t.Fatalf("error body was not reusable: eof=%v closed=%v", body.eof, body.closed)
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

func TestDevLoginNotMountedByDefault(t *testing.T) {
	// The offline dev provider must not exist unless explicitly enabled;
	// a mounted-by-accident dev login in production is an auth bypass.
	h := &Handler{Service: &Service{}}
	mux := http.NewServeMux()
	h.Mount(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/dev/login", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("dev login reachable without DevLogin: got %d", w.Code)
	}

	h.DevLogin = true
	mux = http.NewServeMux()
	h.Mount(mux)
	w = httptest.NewRecorder()
	// With no database wired the handler fails, but the route must exist —
	// anything but 404 proves the gate opened.
	func() {
		defer func() { _ = recover() }() // nil pool panics; only routing matters here
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/dev/login", nil))
	}()
	if w.Code == http.StatusNotFound {
		t.Fatal("dev login not mounted with DevLogin=true")
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

func TestOptionalSessionWithoutCookiePassesThrough(t *testing.T) {
	// Public routes (search, skill detail) must stay reachable with no session
	// at all — OptionalSession must never reject (ADR-020, DISC-010).
	h := &Handler{Service: &Service{}}
	called := false
	var gotUser bool
	next := h.OptionalSession(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, gotUser = SessionUser(r.Context())
	})
	w := httptest.NewRecorder()
	next(w, httptest.NewRequest(http.MethodGet, "/skills/search", nil))
	if w.Code != http.StatusOK || !called {
		t.Fatalf("want handler called with 200, got %d (called=%v)", w.Code, called)
	}
	if gotUser {
		t.Fatal("want no user in context without a session cookie")
	}
}
