package packaging

import (
	"strings"
	"testing"
)

func TestAReleaseDeclarationFillsOnlyALicenseTheVersionLacks(t *testing.T) {
	mit, apache, manifestTier := "MIT", "Apache-2.0", "manifest"
	cases := []struct {
		name           string
		version        VersionFacts
		declared       string
		wantExpression *string
		wantTier       string
		wantDisclosed  bool
	}{
		{"no own license, declared on release", VersionFacts{}, mit, &mit, "release-declaration", true},
		{"own license and a different declaration", VersionFacts{LicenseExpression: &apache, LicenseSource: &manifestTier}, mit, &apache, "manifest", false},
		{"own license, nothing declared", VersionFacts{LicenseExpression: &apache, LicenseSource: &manifestTier}, "", &apache, "manifest", false},
		{"neither", VersionFacts{}, "", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withReleaseDeclaration(tc.version, tc.declared)
			if (got.LicenseExpression == nil) != (tc.wantExpression == nil) ||
				got.LicenseExpression != nil && *got.LicenseExpression != *tc.wantExpression {
				t.Errorf("expression = %v, want %v", got.LicenseExpression, tc.wantExpression)
			}
			tier := ""
			if got.LicenseSource != nil {
				tier = *got.LicenseSource
			}
			if tier != tc.wantTier {
				t.Errorf("source tier = %q, want %q", tier, tc.wantTier)
			}
			disclosures := releaseDeclarationDisclosures(got)
			if disclosed := len(disclosures) == 1 && disclosures[0].Code == "license-from-release-declaration"; disclosed != tc.wantDisclosed {
				t.Errorf("disclosures = %+v, want disclosed %v", disclosures, tc.wantDisclosed)
			}
			if note := releaseDeclarationInstallNote(got); strings.Contains(note, "The publisher declared") != tc.wantDisclosed {
				t.Errorf("install note = %q, want present %v", note, tc.wantDisclosed)
			}
		})
	}
}
