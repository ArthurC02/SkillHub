package catalog

import (
	"encoding/json"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

func scanJSON(t *testing.T, warnings int, codes ...string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"warnings": warnings, "codes": codes})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// DISC-004 不得自行推定為通過: a row the projection has no scan for reports
// unknown. The tempting default — zero warnings, no flags — is indistinguishable
// from a package that was scanned and found clean.
func TestRiskHintReportsMissingScanAsUnknown(t *testing.T) {
	for name, stored := range map[string][]byte{
		"absent":      nil,
		"not json":    []byte("nonsense"),
		"json 'null'": []byte("null"),
	} {
		t.Run(name, func(t *testing.T) {
			got := riskHint(stored)
			if name != "json 'null'" && got.ScanStatus != "unavailable" {
				t.Fatalf("scan_status = %q, want unavailable", got.ScanStatus)
			}
			if got.Warnings != 0 || got.HasScripts {
				t.Fatalf("unknown scan invented findings: %+v", got)
			}
		})
	}
}

func TestRiskHintLevels(t *testing.T) {
	tests := []struct {
		name     string
		stored   []byte
		want     string
		warnings int
	}{
		{"warnings dominate", scanJSON(t, 2, "embedded-script", "script-file"), riskLevelWarning, 2},
		{"disclosures only", scanJSON(t, 0, "script-file", "external-url"), riskLevelDisclosed, 0},
		{"nothing to disclose", scanJSON(t, 0, "license-from-manifest-reference"), riskLevelNone, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := riskHint(tc.stored)
			if got.ScanStatus != "scanned" {
				t.Fatalf("scan_status = %q, want scanned", got.ScanStatus)
			}
			if got.Level != tc.want {
				t.Fatalf("level = %q, want %q (%+v)", got.Level, tc.want, got)
			}
			if got.Warnings != tc.warnings {
				t.Fatalf("warnings = %d, want %d", got.Warnings, tc.warnings)
			}
		})
	}
}

// The projection stores the tag buckets; only the dependency one is shown on a
// result row (DISC-002 依賴).
func TestDependencyTagsReadOnlyTheDependencyBucket(t *testing.T) {
	stored, err := json.Marshal(map[string][]string{
		"inputs":       {"pdf"},
		"outputs":      {"csv"},
		"tools":        {"bash"},
		"dependencies": {"poppler", "pandas"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := dependencyTags(stored)
	if len(got) != 2 || got[0] != "poppler" || got[1] != "pandas" {
		t.Fatalf("dependencies = %v, want only the dependency bucket", got)
	}
	// Never nil: an omitted list reads as "this skill has no dependencies",
	// which is not what a pending enrichment means.
	for name, stored := range map[string][]byte{"absent": nil, "empty object": []byte(`{}`)} {
		if got := dependencyTags(stored); got == nil || len(got) != 0 {
			t.Fatalf("%s: dependencies = %v, want an empty list", name, got)
		}
	}
}

// spec_validation is derived from "does a version exist", because a package that
// fails static validation is never stored. capability and runtime must stay
// unverified regardless — that is DISC-002's 尚未試跑.
func TestResultFacetsDeriveCompatibilityFromVersionPresence(t *testing.T) {
	var withVersion searchResult
	resultFacets(&withVersion, nil, nil, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true})
	if withVersion.Compat.SpecValidation != "passed" {
		t.Fatalf("spec_validation = %q for an indexed version", withVersion.Compat.SpecValidation)
	}
	if withVersion.VerifiedAt == "" {
		t.Fatal("verified_at empty despite a version timestamp")
	}

	var noVersion searchResult
	resultFacets(&noVersion, nil, nil, pgtype.Timestamptz{})
	if noVersion.Compat.SpecValidation != "unverified" {
		t.Fatalf("spec_validation = %q for a skill with no version", noVersion.Compat.SpecValidation)
	}
	if noVersion.VerifiedAt != "" {
		t.Fatalf("verified_at = %q with no version to have verified", noVersion.VerifiedAt)
	}

	for _, r := range []searchResult{withVersion, noVersion} {
		if r.Compat.Capability != "unverified" || r.Compat.Runtime != "unverified" {
			t.Fatalf("sandbox axes claimed a verdict before M2: %+v", r.Compat)
		}
		if r.Tier.Value != string(TierIndexed) {
			t.Fatalf("tier = %q, want indexed", r.Tier.Value)
		}
	}
}

// DISC-003 限制, scan half: the package's own contents imply requirements
// regardless of what its document claims, and each finding code contributes at
// most one line however many findings carry it.
func TestScanDerivedLimitationsAreDeduplicatedAndLabelled(t *testing.T) {
	report := skillpkg.Validate(fstest.MapFS{
		"SKILL.md": {Data: []byte("---\nname: demo-skill\ndescription: demo\nlicense: MIT\n---\n\n" +
			"See https://example.com/a and https://example.com/b.\n")},
		"scripts/run.py":   {Data: []byte("print('hi')\n")},
		"scripts/other.py": {Data: []byte("print('hi')\n")},
	})

	got := scanDerivedLimitations(report)
	seen := map[string]int{}
	for _, l := range got {
		if l.Source != limitSourceScan {
			t.Fatalf("scan-derived limitation labelled %q", l.Source)
		}
		seen[l.Text]++
	}
	if len(got) == 0 {
		t.Fatal("a package with scripts and external URLs produced no limitations")
	}
	for text, n := range seen {
		if n > 1 {
			t.Fatalf("limitation repeated %d times: %q", n, text)
		}
	}
	if got[0].Text == "" {
		t.Fatal("empty limitation text")
	}
}

// The model half is labelled too, so a reader can tell the author's documented
// limits from what the platform inferred from the package (ADR-013).
func TestModelLimitationsAreLabelledAndSplitPerLine(t *testing.T) {
	got := modelLimitations("無法處理加密的 PDF。\n\n需要 OpenAI API key。\n")
	if len(got) != 2 {
		t.Fatalf("limitations = %+v, want one per non-empty line", got)
	}
	for _, l := range got {
		if l.Source != limitSourceModel {
			t.Fatalf("model limitation labelled %q", l.Source)
		}
	}
	if n := len(modelLimitations("")); n != 0 {
		t.Fatalf("empty enrichment produced %d limitations", n)
	}
}
