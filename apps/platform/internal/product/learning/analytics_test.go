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

func TestPurgeExpiredRejectsMissingConfiguration(t *testing.T) {
	for name, svc := range map[string]*Service{
		"nil service": nil,
		"nil pool":    {Retention: time.Hour},
		"zero window": {},
		"negative":    {Retention: -time.Hour},
	} {
		if _, err := svc.PurgeExpired(context.Background()); err == nil {
			t.Errorf("%s: purge reported success", name)
		}
	}
}

// The one thing a search event records about the words themselves. A bucket, not
// a locale and not the text: ADR-013's vector leg is meant to carry cross-script
// intent, and this is the coarsest signal that can tell whether it is being asked
// to (ADR-029 決策 2).
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

// Nothing is collected until a retention period exists (NFR-002, ADR-029 決策 5),
// and a nil service is one of the ways a deployment says so.
func TestCollectionIsOffWithoutARetentionPeriod(t *testing.T) {
	var nilSvc *Service
	if nilSvc.Enabled() {
		t.Error("a nil service reports itself as collecting")
	}
	if (&Service{}).Enabled() {
		t.Error("a service with no pool and no retention reports itself as collecting")
	}
}

// 04 丙-57: a session id this request just minted is not this request's id.
//
// The platform does not serve the SPA's document, so a visitor's first API calls
// arrive together and every one of them is cold. Each would mint an id, each
// would set a cookie, the browser keeps exactly one, and the events written by
// the rest are attached to ids no later request will ever carry. On a deep link
// that strands a search_performed on an id that can never acquire a
// skill_detail_viewed - 01 §11.2's first segment measured against a denominator
// it cannot convert.
//
// The two exactly-once assertions added alongside the fix cannot cover this: an
// integration test drives one sequential cookie jar and so cannot produce
// concurrent cold requests at all. This is the test that actually holds the fix
// down (adversarial review, 2026-08-24).
//
// The pool never connects and never needs to. Enabled() only checks that one
// exists, and every write goes through emit(), which drops anything with no
// session id before it reaches the database - which is precisely the behaviour
// under test.
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

	// Cold: no cookie on the way in.
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
			// Setting it here would mark the day as started on behalf of a
			// request whose session_started was never written, and the marker
			// would then stop the next request from writing one either.
			t.Error("a cold request marked the visit as started")
		}
	}
	if len(minted) != 32 {
		t.Fatalf("no session cookie was offered to the browser: %q", minted)
	}

	// Warm: the browser accepted it and sent it back. Now it is this request's
	// id, and now the day's marker may be set.
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
