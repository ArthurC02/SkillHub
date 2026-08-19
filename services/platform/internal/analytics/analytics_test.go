package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// Both arrival attributes come from the client, so both are clamped rather than
// trusted: a crafted link must not be able to write anything the schema did not
// declare (0029's CHECK is the second half of the same rule).
func TestArrivalIsClampedToTheWhitelist(t *testing.T) {
	cases := []struct {
		query       string
		wantArrival string
		wantRank    int
	}{
		{"", "direct", 0},
		{"?from=direct", "direct", 0},
		{"?from=newsletter&rank=3", "direct", 0}, // not in the whitelist
		{"?from=search&rank=2", "search", 2},
		{"?from=search", "search", 0},
		{"?from=search&rank=0", "search", 0},
		{"?from=search&rank=-1", "search", 0},
		{"?from=search&rank=99999", "search", 0}, // out of range, dropped not stored
		{"?from=search&rank=abc", "search", 0},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/api/skills/x"+tc.query, nil)
		arrival, rank := ArrivalFromRequest(r)
		if arrival != tc.wantArrival || rank != tc.wantRank {
			t.Errorf("%q -> (%q, %d), want (%q, %d)", tc.query, arrival, rank, tc.wantArrival, tc.wantRank)
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
