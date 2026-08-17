package analytics

import (
	"net/http/httptest"
	"testing"
)

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
