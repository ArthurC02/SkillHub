package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// A nil transaction is deliberate: every case here must be rejected before the
// function reaches the database, and a nil pointer dereference is a louder
// failure than a passing assertion would be.
func TestImportWritesRejectUnvalidatedReports(t *testing.T) {
	ctx := context.Background()
	cases := map[string]skillpkg.Report{
		"zero report has no manifest": {},
		"blocked report": {
			Manifest: &skillpkg.Manifest{Name: "x", Description: "y"},
			Blocked:  true,
		},
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := CreateSkillFromPackage(ctx, nil, pgtype.UUID{}, report, ""); !errors.Is(err, ErrUnvalidatedPackage) {
				t.Errorf("CreateSkillFromPackage err = %v", err)
			}
			if _, err := CreateVersionFromPackage(ctx, nil, NewVersion{Report: report}); !errors.Is(err, ErrUnvalidatedPackage) {
				t.Errorf("CreateVersionFromPackage err = %v", err)
			}
			if err := UpdateSummaryFromPackage(ctx, nil, pgtype.UUID{}, pgtype.UUID{}, report); !errors.Is(err, ErrUnvalidatedPackage) {
				t.Errorf("UpdateSummaryFromPackage err = %v", err)
			}
		})
	}
}

func TestVersionLicenseKeepsProvenanceTier(t *testing.T) {
	if expression, source := versionLicense(skillpkg.Report{}); expression != nil || source != nil {
		t.Errorf("unresolved license = %v/%v, want NULL in both columns", expression, source)
	}
	expression, source := versionLicense(skillpkg.Report{
		LicenseExpression: "MIT",
		LicenseSource:     "repo-file",
	})
	if expression == nil || *expression != "MIT" {
		t.Errorf("expression = %v", expression)
	}
	// The tier is what stops a repo-level file from reading as a declaration
	// the package made about itself (ADR-021).
	if source == nil || *source != "repo-file" {
		t.Errorf("source = %v", source)
	}
}

// WS-001 lets a user fork a skill; it does not say once. The fixed `-fork`
// suffix meant the second fork hit the unique index and the user got a 409 with
// no next step — for what is the ordinary shape of the work (試跑、改、再試一份).
//
// The suffix series is also what keeps a fork of a fork from becoming
// `x-fork-fork`: the base is the source name with any existing fork suffix
// trimmed off, so the series stays flat.
func TestForkOrdinalParsingKeepsTheSeriesFlat(t *testing.T) {
	for _, tc := range []struct {
		in       string
		wantBase string
		wantOK   bool
	}{
		{"tidy-csv-fork-2", "tidy-csv", true},
		{"tidy-csv-fork-10", "tidy-csv", true},
		{"tidy-csv-fork", "tidy-csv-fork", false}, // handled by the plain trim
		{"tidy-csv", "tidy-csv", false},
		{"tidy-csv-fork-0", "tidy-csv-fork-0", false}, // not an ordinal we write
		{"tidy-csv-fork-1", "tidy-csv-fork-1", false}, // nor is 1: that name is `-fork`
		{"tidy-csv-fork-x", "tidy-csv-fork-x", false}, // somebody's own name
		{"-fork-2", "-fork-2", false},                 // no base to keep
	} {
		base, _, ok := cutForkOrdinal(tc.in)
		if base != tc.wantBase || ok != tc.wantOK {
			t.Errorf("cutForkOrdinal(%q) = (%q, %v), want (%q, %v)", tc.in, base, ok, tc.wantBase, tc.wantOK)
		}
	}
}
