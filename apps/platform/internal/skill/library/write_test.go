package registry

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestImportWritesRejectUnvalidatedReports(t *testing.T) {
	cases := map[string]skillpkg.Report{
		"zero report has no manifest": {},
		"blocked report": {
			Manifest: &skillpkg.Manifest{Name: "x", Description: "y"},
			Blocked:  true,
		},
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SkillFromPackage(pgtype.UUID{}, report, ""); !errors.Is(err, ErrUnvalidatedPackage) {
				t.Errorf("SkillFromPackage err = %v", err)
			}
			if _, err := ContentFromPackage(NewVersion{Report: report}, false); !errors.Is(err, ErrUnvalidatedPackage) {
				t.Errorf("ContentFromPackage err = %v", err)
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
		LicenseSource:     skillpkg.LicenseSourceRepoFile,
	})
	if expression == nil || *expression != "MIT" {
		t.Errorf("expression = %v", expression)
	}

	if source == nil || *source != string(skillpkg.LicenseSourceRepoFile) {
		t.Errorf("source = %v", source)
	}
}

func TestForkOrdinalParsingKeepsTheSeriesFlat(t *testing.T) {
	for _, tc := range []struct {
		in       string
		wantBase string
		wantOK   bool
	}{
		{"tidy-csv-fork-2", "tidy-csv", true},
		{"tidy-csv-fork-10", "tidy-csv", true},
		{"tidy-csv-fork", "tidy-csv-fork", false},
		{"tidy-csv", "tidy-csv", false},
		{"tidy-csv-fork-0", "tidy-csv-fork-0", false},
		{"tidy-csv-fork-1", "tidy-csv-fork-1", false},
		{"tidy-csv-fork-x", "tidy-csv-fork-x", false},
		{"-fork-2", "-fork-2", false},
	} {
		base, _, ok := cutForkOrdinal(tc.in)
		if base != tc.wantBase || ok != tc.wantOK {
			t.Errorf("cutForkOrdinal(%q) = (%q, %v), want (%q, %v)", tc.in, base, ok, tc.wantBase, tc.wantOK)
		}
	}
}
