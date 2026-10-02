package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixedIdentityProvider struct{ identity ExternalIdentity }

func (fixedIdentityProvider) AuthURL(string) string                            { return "" }
func (fixedIdentityProvider) Exchange(context.Context, string) (string, error) { return "tok", nil }
func (f fixedIdentityProvider) Identify(context.Context, string) (ExternalIdentity, error) {
	return f.identity, nil
}

func signupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping sign-up integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	})
	return pool
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string, deleted bool) {
	t.Helper()
	deletedAt := "NULL"
	if deleted {
		deletedAt = "now()"
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (email, display_name, deleted_at) VALUES ($1, 'fixture', `+deletedAt+`)`, email); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email = $1 AND display_name = 'fixture'`, email)
	})
}

func githubIdentity(email string) ExternalIdentity {
	return ExternalIdentity{
		Provider: providerGitHub, ProviderUserID: uuid.NewString(),
		Email: email, Name: "Newcomer", Login: "newcomer",
	}
}

func TestASignupWhoseEmailBelongsToALiveAccountIsRefusedAsEmailTaken(t *testing.T) {
	pool := signupPool(t)
	taken := uuid.NewString() + "@example.test"
	seedUser(t, pool, taken, false)
	s := &Service{Pool: pool}

	for _, email := range []string{taken, strings.ToUpper(taken), "  " + taken + " "} {
		if _, err := s.LoginOrSignup(context.Background(), githubIdentity(email)); !errors.Is(err, ErrEmailTaken) {
			t.Errorf("sign-up with email %q: error = %v, want ErrEmailTaken", email, err)
		}
	}
}

func TestASignupWhoseEmailOnlyAGoneAccountHoldsSucceeds(t *testing.T) {
	pool := signupPool(t)
	reused := uuid.NewString() + "@example.test"
	seedUser(t, pool, reused, true)
	s := &Service{Pool: pool}

	token, err := s.LoginOrSignup(context.Background(), githubIdentity(reused))
	if err != nil || token == "" {
		t.Fatalf("sign-up reusing a deleted account's email: token %q, error %v; want a session", token, err)
	}
}

func TestTheCallbackAnswersAnEmailTakenSignupWith409AndAPlainMessage(t *testing.T) {
	pool := signupPool(t)
	taken := uuid.NewString() + "@example.test"
	seedUser(t, pool, taken, false)
	h := &Handler{Service: &Service{Pool: pool, OAuth: fixedIdentityProvider{githubIdentity(taken)}}}
	r := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=abc", nil)
	r.AddCookie(&http.Cookie{Name: stateCookie, Value: "abc"})
	w := httptest.NewRecorder()

	h.finishLogin(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("callback = %d (%s), want 409", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "這個 email 已經被另一個帳號使用") {
		t.Errorf("body = %q, want the plain Chinese explanation", w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			t.Errorf("a refused sign-up still set a session cookie")
		}
	}
}
