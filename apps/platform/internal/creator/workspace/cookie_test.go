package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The session cookie's three attributes, pinned.
//
// SameSite=Lax is this platform's ONLY CSRF defence on sixty-odd mutating
// routes — there is no token anywhere, deliberately — and until now no test
// asserted its value. It is one word in one struct literal, and changing it to
// None (which somebody will want the first time an embed is asked for) removes
// that defence from every route at once with the entire suite still green.
//
// HttpOnly is what keeps the token out of reach of script; Secure is what keeps
// it off the wire in plaintext, and it follows the handler's own flag so a
// deployment on plain http (COOKIE_INSECURE=1, local dev) still works.
func TestTheSessionCookiePinsItsSecurityAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		rec := httptest.NewRecorder()
		(&Handler{Secure: secure}).setSessionCookie(rec, "a-token")

		c := sessionCookieFrom(t, rec.Result())
		if c.SameSite != http.SameSiteLaxMode {
			t.Errorf("Secure=%v: SameSite = %v, want Lax; it is the only CSRF defence on every mutating route", secure, c.SameSite)
		}
		if !c.HttpOnly {
			t.Errorf("Secure=%v: HttpOnly is off; the session token became readable by script", secure)
		}
		if c.Secure != secure {
			t.Errorf("Secure=%v: cookie Secure = %v; it must follow the handler's flag, not a constant", secure, c.Secure)
		}
		if c.Path != "/" {
			t.Errorf("Secure=%v: Path = %q, want /", secure, c.Path)
		}
		if want := int(SessionTTL / time.Second); c.MaxAge != want {
			t.Errorf("Secure=%v: MaxAge = %d, want %d (SessionTTL)", secure, c.MaxAge, want)
		}
	}
}

// Logging out must clear the cookie with the same attributes it was set with: a
// browser matches on name, path and the secure flag when deciding what a
// Set-Cookie replaces, so a clear that disagrees leaves the original in place.
func TestTheLogoutCookieClearsWithTheSameAttributes(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Handler{Secure: true}).logout(rec, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

	c := sessionCookieFrom(t, rec.Result())
	if c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("logout wrote value=%q MaxAge=%d; it must expire the cookie", c.Value, c.MaxAge)
	}
	if c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("logout cookie attributes differ from the one it replaces: %+v", c)
	}
}

func sessionCookieFrom(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatalf("no %s cookie was set", sessionCookie)
	return nil
}
