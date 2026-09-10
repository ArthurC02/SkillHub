package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestPublicSearchDoesNotReportDatabaseFailureAsNoResults(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://unused:unused@127.0.0.1:1/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=spreadsheet", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	(&Handler{Svc: &Service{Pool: pool}}).PublicSearch(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("database failure returned %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"no_results":true`) {
		t.Fatalf("database failure was also reported as no results: %s", rec.Body.String())
	}
}

func TestPublicSearchRejectsOversizedQueryBeforeLLMOrDatabase(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/skills/search?q="+strings.Repeat("x", 2001),
		nil,
	)
	rec := httptest.NewRecorder()
	(&Handler{Svc: &Service{}}).PublicSearch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized query returned %d, want 400; body=%s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "搜尋文字最多 2000 字") {
		t.Errorf("the refusal is not the Chinese sentence: %s", rec.Body.String())
	}
}

func scanJSON(t *testing.T, warnings int, codes ...string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"warnings": warnings, "codes": codes})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

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
			if got.Warnings != 0 || len(got.Disclosures) != 0 {
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

	for name, stored := range map[string][]byte{"absent": nil, "empty object": []byte(`{}`)} {
		if got := dependencyTags(stored); got == nil || len(got) != 0 {
			t.Fatalf("%s: dependencies = %v, want an empty list", name, got)
		}
	}
}

func TestResultFacetsDeriveCompatibilityFromVersionPresence(t *testing.T) {
	unmeasured := measuredCompat("unverified", "unverified", "", pgtype.Timestamptz{})

	var withVersion searchResult
	resultFacets(&withVersion, string(TierIndexed), nil, nil, nil, nil, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, unmeasured)
	if withVersion.Compat.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q for an indexed version", withVersion.Compat.SpecValidation)
	}
	if withVersion.VerifiedAt == "" {
		t.Fatal("verified_at empty despite a version timestamp")
	}

	var noVersion searchResult
	resultFacets(&noVersion, string(TierIndexed), nil, nil, nil, nil, pgtype.Timestamptz{}, unmeasured)
	if noVersion.Compat.SpecValidation.Value != "unverified" {
		t.Fatalf("spec_validation = %q for a skill with no version", noVersion.Compat.SpecValidation)
	}
	if noVersion.VerifiedAt != "" {
		t.Fatalf("verified_at = %q with no version to have verified", noVersion.VerifiedAt)
	}

	for _, r := range []searchResult{withVersion, noVersion} {
		if r.Compat.Capability.Value != "unverified" || r.Compat.Runtime.Value != "unverified" {
			t.Fatalf("sandbox axes claimed a verdict nothing measured: %+v", r.Compat)
		}
		if r.Compat.Note != compatUnverifiedNote {
			t.Fatalf("unmeasured row carried the measured note: %q", r.Compat.Note)
		}
		if r.Tier.Value != string(TierIndexed) {
			t.Fatalf("tier = %q, want indexed", r.Tier.Value)
		}
	}
}

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

func TestResultFacetsCarryTheMeasuredAgentAxis(t *testing.T) {
	var r searchResult
	measured := measuredCompat("activated", "transpiled", "skillhub/runtime-agent-sdk:2026.08-1",
		pgtype.Timestamptz{Time: time.Unix(1_755_000_000, 0), Valid: true})
	resultFacets(&r, string(TierIndexed), nil, nil, nil, nil, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, measured)

	if r.Compat.Capability.Value != "activated" || r.Compat.Runtime.Value != "transpiled" {
		t.Fatalf("measured verdict lost on the way to the row: %+v", r.Compat)
	}
	if r.Compat.RuntimeImage == "" || r.Compat.MeasuredAt == "" {
		t.Fatalf("verdict reported without the image or the time it was measured: %+v", r.Compat)
	}
	if r.Compat.Note != compatMeasuredNote {
		t.Fatalf("measured row carried the unverified note: %q", r.Compat.Note)
	}

	if r.Compat.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q, want passed", r.Compat.SpecValidation)
	}
}

func TestParseFiltersAgentRuntime(t *testing.T) {
	for _, v := range []string{"native", "transpiled", "failed", "unverified"} {
		f, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x&agent="+v, nil))
		if err != nil {
			t.Fatalf("agent=%s rejected: %v", v, err)
		}
		if f.AgentRuntime == nil || *f.AgentRuntime != v || !f.active() {
			t.Fatalf("agent=%s did not reach the filter set: %+v", v, f)
		}
	}
	if _, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x&agent=claude", nil)); err == nil {
		t.Fatal("agent=claude accepted; an unknown value must not be silently dropped")
	}
	f, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?q=x", nil))
	if err != nil || f.AgentRuntime != nil || f.active() {
		t.Fatalf("absent agent filter did not stay absent: %+v (%v)", f, err)
	}
}

func TestParseFiltersRejectsExplicitEmptyValues(t *testing.T) {
	for _, name := range []string{"script", "validation", "agent", "tier"} {
		if _, err := parseFilters(httptest.NewRequest(http.MethodGet, "/?"+name+"=", nil)); err == nil {
			t.Errorf("explicitly empty %s filter was treated as absent", name)
		}
	}
}

func TestAnUnavailableScanIsUnknownAndNotTheLowestRisk(t *testing.T) {
	for name, scan := range map[string][]byte{
		"no scan at all":     nil,
		"empty":              {},
		"not parseable JSON": []byte("{not json"),
	} {
		got := riskHint(scan)
		if got.ScanStatus != "unavailable" {
			t.Errorf("%s: scan_status = %q, want unavailable", name, got.ScanStatus)
		}
		if got.Level != riskLevelUnknown {
			t.Errorf("%s: level = %q, want %q", name, got.Level, riskLevelUnknown)
		}
		if got.Level == riskLevelNone {
			t.Errorf("%s: 「沒有掃描紀錄」 must not share a value with 「掃過了,沒事」", name)
		}
		if got.Disclosures == nil {
			t.Errorf("%s: disclosures must serialise as [] rather than null", name)
		}
	}

	if got := riskHint(scanJSON(t, 0, "license-from-manifest-reference")); got.Level != riskLevelNone {
		t.Errorf("a scan that found nothing to disclose = %q, want %q", got.Level, riskLevelNone)
	}
}
