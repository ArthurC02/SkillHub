package wiring

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

func versionID(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

func docketOf(n int) []publishing.DocketEntry {
	docket := make([]publishing.DocketEntry, n)
	for i := range docket {
		docket[i] = publishing.DocketEntry{State: publishing.ExposureState{Publisher: "acme", Name: "skill"}}
	}
	return docket
}

func TestTheExposureToolShowsAtMostTwentyAndSaysHowManyWait(t *testing.T) {
	for _, tc := range []struct {
		waiting, shown int
	}{{0, 0}, {20, 20}, {21, 20}} {
		facts := NewExposureFacts(docketOf(tc.waiting))
		if facts.Waiting != tc.waiting || len(facts.Publications) != tc.shown {
			t.Errorf("%d waiting: got waiting %d, %d shown; want %d shown", tc.waiting, facts.Waiting, len(facts.Publications), tc.shown)
		}
	}
	if facts := NewExposureFacts(nil); facts.Publications == nil {
		t.Error("an empty queue answers null publications, which the agent cannot cite")
	}
}

func TestTheExposureToolCarriesScanErrorsAndWarningsButNotNotes(t *testing.T) {
	entry := publishing.DocketEntry{
		State: publishing.ExposureState{Publisher: "acme", Name: "csv", Approved: true},
		Findings: skillpkg.CategorizedFindings{
			Errors:   []skillpkg.Finding{{Severity: skillpkg.SeverityError, Code: "entry-path-escape"}},
			Warnings: []skillpkg.Finding{{Severity: skillpkg.SeverityWarning, Code: "external-url"}},
			Infos:    []skillpkg.Finding{{Severity: skillpkg.SeverityInfo, Code: "dependency-file"}},
		},
	}
	got := NewExposureFacts([]publishing.DocketEntry{entry}).Publications[0]
	if got.Address != "acme/csv" || !got.PreviouslyApproved {
		t.Errorf("entry = %+v, want acme/csv previously approved", got)
	}
	if len(got.ScanFindings) != 2 || got.ScanFindings[0].Code != "entry-path-escape" || got.ScanFindings[1].Code != "external-url" {
		t.Errorf("scan findings = %+v, want the error then the warning", got.ScanFindings)
	}
	if got.SearchText != nil {
		t.Errorf("search text without a snapshot = %+v, want null", got.SearchText)
	}
	clean := NewExposureFacts(docketOf(1)).Publications[0]
	if clean.ScanFindings == nil || clean.PreviouslyApproved {
		t.Errorf("a clean, never-reviewed entry = %+v, want an empty finding list and not previously approved", clean)
	}
}

func TestTheExposureToolSaysWhetherTheSearchTextIsTheReleasedVersions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot pgtype.UUID
		want     bool
	}{{"same version", versionID(1), true}, {"older version", versionID(2), false}} {
		entry := publishing.DocketEntry{
			State:    publishing.ExposureState{VersionID: versionID(1)},
			Snapshot: &publishing.SearchSnapshot{VersionID: tc.snapshot, Summary: "Cleans CSV files."},
		}
		text := NewExposureFacts([]publishing.DocketEntry{entry}).Publications[0].SearchText
		if text == nil || text.MatchesRelease != tc.want || text.Summary != "Cleans CSV files." {
			t.Errorf("%s: search text = %+v, want matches_release %v", tc.name, text, tc.want)
		}
	}
}

func TestSearchTextIsClippedAtSixHundredCharacters(t *testing.T) {
	atLimit := strings.Repeat("字", searchTextRunes)
	if got := clip(atLimit); got != atLimit {
		t.Errorf("text at the limit was changed to %d runes", utf8.RuneCountInString(got))
	}
	over := atLimit + "x"
	if got := clip(over); got != atLimit+"…" {
		t.Errorf("text one past the limit became %d runes, want the limit plus an ellipsis", utf8.RuneCountInString(got))
	}
}
