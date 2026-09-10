package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVisitBoundaryRecognizesANewUTCDate(t *testing.T) {
	now := time.Date(2026, 8, 19, 23, 30, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !newVisit(req, now) {
		t.Fatal("a request without a visit cookie was not a new visit")
	}
	req.AddCookie(&http.Cookie{Name: visitCookie, Value: "2026-08-19"})
	if newVisit(req, now) {
		t.Fatal("a second request on the same UTC day became another visit")
	}
	if !newVisit(req, now.Add(time.Hour)) {
		t.Fatal("the first request on the next UTC day was not a new visit")
	}
}

func TestVisitCookieNeverOutlivesTheAnalyticsSession(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if got := visitLifetimeSeconds(now, time.Hour); got != 3600 {
		t.Errorf("visit cookie lifetime = %d seconds, want the one-hour session retention", got)
	}
	if got := visitLifetimeSeconds(now, 48*time.Hour); got != 12*60*60 {
		t.Errorf("visit cookie lifetime = %d seconds, want until UTC midnight", got)
	}
}

func TestQueryScriptBuckets(t *testing.T) {
	cases := map[string]string{
		"summarise a csv": "latin",
		"整理這份試算表":         "han",
		"整理 csv":          "mixed",
		"":                "other",
		"1234 567":        "other",
		"まとめて":            "han",
		"요약해 주세요":         "han",
	}
	for query, want := range cases {
		if got := queryScript(query); got != want {
			t.Errorf("queryScript(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestCollectionIsOffWithoutARetentionPeriod(t *testing.T) {
	var nilSvc *Service
	if nilSvc.Enabled() {
		t.Error("a nil service reports itself as collecting")
	}
	if (&Service{}).Enabled() {
		t.Error("a service with no pool and no retention reports itself as collecting")
	}
}

func TestAFreshlyMintedSessionIdIsOfferedNotUsed(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := &Service{Pool: pool, Retention: 180 * 24 * time.Hour}

	var seen string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = SessionID(r.Context())
	})

	cold := httptest.NewRecorder()
	svc.Sessions(next).ServeHTTP(cold, httptest.NewRequest(http.MethodGet, "/api/skills/search?q=x", nil))

	if seen != "" {
		t.Errorf("a cold request carried the id it had just minted: %q", seen)
	}
	cookies := cold.Result().Cookies()
	minted := ""
	for _, c := range cookies {
		switch c.Name {
		case sessionCookie:
			minted = c.Value
		case visitCookie:

			t.Error("a cold request marked the visit as started")
		}
	}
	if len(minted) != 32 {
		t.Fatalf("no session cookie was offered to the browser: %q", minted)
	}

	warm := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=x", nil)
	warm.AddCookie(&http.Cookie{Name: sessionCookie, Value: minted})
	rec := httptest.NewRecorder()
	svc.Sessions(next).ServeHTTP(rec, warm)

	if seen != minted {
		t.Errorf("a request carrying a session cookie was handled as %q, want %q", seen, minted)
	}
	var visitSet bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == visitCookie {
			visitSet = true
		}
	}
	if !visitSet {
		t.Error("the first confirmed request of the day did not start a visit")
	}
}
