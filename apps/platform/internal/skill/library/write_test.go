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
			if _, err := CreateSkillFromPackage(ctx, nil, pgtype.UUID{}, report); !errors.Is(err, ErrUnvalidatedPackage) {
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
