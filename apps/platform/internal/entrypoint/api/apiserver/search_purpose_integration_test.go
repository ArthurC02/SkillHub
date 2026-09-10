package apiserver_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

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

func TestPurposeOtherThanReferenceIs400(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "purpose-invalid")

	if code := c.status(t, http.MethodGet, "/api/skills/search?q=abc&purpose=other"); code != http.StatusBadRequest {
		t.Fatalf("purpose=other: got %d, want 400", code)
	}
}
