// ADR-066's `purpose=reference` on the public search endpoint: GEN-006's
// reference picker calls GET /api/skills/search like any other caller, but it
// is not a DISC-001 search intent, so it must write no search_performed
// funnel event (and, though this file cannot observe it directly, no
// match-reason model call — see discovery.Service.Search's silent parameter).
//
// Shared harness (TestMain, requireDB, newAPI, login, betaAPI, newFixture,
// betaCount) lives in authz_integration_test.go, run_integration_test.go and
// beta_integration_test.go.
package apiserver_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

// The event stays at 0 for a purpose=reference call in a session that has made
// exactly one such call, and reaches 1 for an ordinary search in the same
// session right after — proving the difference is the parameter and not some
// property of the session or the query.
func TestPurposeReferenceWritesNoSearchPerformedEvent(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-purpose-reference")
	session := f.analyticsSession(t)
	if session == "" {
		t.Fatal("no analytics session cookie was issued")
	}

	if code := f.status(t, http.MethodGet, "/api/skills/search?q=summarise+a+csv+file&purpose=reference"); code != http.StatusOK {
		t.Fatalf("purpose=reference search: got %d", code)
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'search_performed' AND session_id = $1`,
		session); n != 0 {
		t.Errorf("search_performed events after a purpose=reference search: %d, want 0", n)
	}

	if code := f.status(t, http.MethodGet, "/api/skills/search?q=summarise+a+csv+file"); code != http.StatusOK {
		t.Fatalf("ordinary search: got %d", code)
	}
	if n := betaCount(t, pool,
		`SELECT count(*) FROM analytics_events WHERE event_name = 'search_performed' AND session_id = $1`,
		session); n != 1 {
		t.Errorf("search_performed events after one ordinary search in the same session: %d, want 1", n)
	}
}

// Any value other than "reference" is refused rather than silently ignored —
// the same discipline every other DISC-003 query parameter on this endpoint
// already follows.
func TestPurposeOtherThanReferenceIs400(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "purpose-invalid")

	if code := c.status(t, http.MethodGet, "/api/skills/search?q=abc&purpose=other"); code != http.StatusBadRequest {
		t.Fatalf("purpose=other: got %d, want 400", code)
	}
}
