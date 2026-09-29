package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSkillListingPageMatchesThePublicBounds(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    SkillListingPage
		wantErr bool
	}{
		{name: "defaults", want: SkillListingPage{Limit: 100}},
		{name: "lower limit", query: "?limit=1", want: SkillListingPage{Limit: 1}},
		{name: "upper limit and zero offset", query: "?limit=100&offset=0", want: SkillListingPage{Limit: 100}},
		{name: "largest offset", query: "?offset=2147483647", want: SkillListingPage{Limit: 100, Offset: 2147483647}},
		{name: "limit below range", query: "?limit=0", wantErr: true},
		{name: "limit above range", query: "?limit=101", wantErr: true},
		{name: "empty limit", query: "?limit=", wantErr: true},
		{name: "negative offset", query: "?offset=-1", wantErr: true},
		{name: "offset above int4", query: "?offset=2147483648", wantErr: true},
		{name: "empty offset", query: "?offset=", wantErr: true},
		{name: "non-integer offset", query: "?offset=next", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/skills"+tc.query, nil)
			got, err := parseSkillListingPage(request)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSkillListingPage(%q) = %+v, want an error", tc.query, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSkillListingPage(%q): %v", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("parseSkillListingPage(%q) = %+v, want %+v", tc.query, got, tc.want)
			}
		})
	}
}

func TestVerificationDistinguishesForkFromImport(t *testing.T) {
	at := pgtype.Timestamptz{Time: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC), Valid: true}
	imported := verificationOf(ScanVerification{State: ScanMeasured, ScannedAt: at})
	if imported.Value != "scanned" || imported.ScannedAt == nil {
		t.Fatalf("an imported version is the one case with a real scan time: %+v", imported)
	}
	if *imported.ScannedAt != "2026-08-01T10:00:00Z" {
		t.Errorf("scanned_at = %q", *imported.ScannedAt)
	}

	forked := verificationOf(ScanVerification{State: ScanNotMeasured})
	if forked.Value != "not_measured" {
		t.Errorf("a fork was measured nowhere in this workspace, got %q", forked.Value)
	}
	if forked.ScannedAt != nil {
		t.Errorf("a state with no measurement must carry no timestamp: %q", *forked.ScannedAt)
	}

	older := pgtype.Timestamptz{Time: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	inh := verificationOf(ScanVerification{State: ScanInherited, AncestorName: "PDF Summariser", ScannedAt: older})
	if inh.Value != "scanned" || inh.ScannedAt == nil {
		t.Fatalf("identical bytes carry the ancestor's scan: %+v", inh)
	}
	if *inh.ScannedAt != "2026-07-01T09:00:00Z" {
		t.Errorf("the inherited time is the ancestor's import, not the fork: %q", *inh.ScannedAt)
	}
	if inh.Label == imported.Label {
		t.Error("an inherited scan and a local one must not read as the same provenance")
	}
	if !strings.Contains(inh.Note, "PDF Summariser") {
		t.Errorf("inheriting silently is forbidden; the ancestor is unnamed: %q", inh.Note)
	}

	empty := verificationOf(ScanVerification{State: ScanNotApplicable})
	if empty.Value != "not_applicable" {
		t.Errorf("no version means nothing to scan, got %q", empty.Value)
	}

	for _, v := range []skillVerification{imported, forked, inh, empty} {
		if v.Label == "" || v.Note == "" {
			t.Errorf("state %q has no wording: %+v", v.Value, v)
		}
	}
}

func TestTheDeletionNoteDoesNotPromiseAPurgeNothingPerforms(t *testing.T) {
	for _, banned := range []string{"purge", "grace", "30-day", "30 day", "days", "天後", "寬限", "清除"} {
		if strings.Contains(strings.ToLower(deletionNote), banned) {
			t.Errorf("the note claims a deletion deadline (%q) and nothing in this repo enforces one: %q",
				banned, deletionNote)
		}
	}

	for _, required := range []string{"搜尋", "凍結", "Fork"} {
		if !strings.Contains(deletionNote, required) {
			t.Errorf("the note stopped saying what the deletion covers (%q missing): %q",
				required, deletionNote)
		}
	}
}
