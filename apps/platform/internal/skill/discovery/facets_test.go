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
}

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
// fails static validation is never stored. The two sandbox axes must stay
// unverified when nothing measured them — that is DISC-002's 尚未試跑, and an
// empty runtime image is how "no measurement" arrives from SQL.
func TestResultFacetsDeriveCompatibilityFromVersionPresence(t *testing.T) {
	unmeasured := measuredCompat("unverified", "unverified", "", pgtype.Timestamptz{})

	var withVersion searchResult
	resultFacets(&withVersion, nil, nil, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, unmeasured)
	if withVersion.Compat.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q for an indexed version", withVersion.Compat.SpecValidation)
	}
	if withVersion.VerifiedAt == "" {
		t.Fatal("verified_at empty despite a version timestamp")
	}

	var noVersion searchResult
	resultFacets(&noVersion, nil, nil, pgtype.Timestamptz{}, unmeasured)
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

// The measured half of the DISC-002 Agent axis. Two things must survive from the
// projection to the response, and both have been wrong in the same way before:
// the verdict must not be reported without the image it was measured on, and the
// note must switch, because the unverified note tells a reader the axis has no
// answer while this row has one.
func TestResultFacetsCarryTheMeasuredAgentAxis(t *testing.T) {
	var r searchResult
	measured := measuredCompat("activated", "transpiled", "skillhub/runtime-agent-sdk:2026.08-1",
		pgtype.Timestamptz{Time: time.Unix(1_755_000_000, 0), Valid: true})
	resultFacets(&r, nil, nil, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, measured)

	if r.Compat.Capability.Value != "activated" || r.Compat.Runtime.Value != "transpiled" {
		t.Fatalf("measured verdict lost on the way to the row: %+v", r.Compat)
	}
	if r.Compat.RuntimeImage == "" || r.Compat.MeasuredAt == "" {
		t.Fatalf("verdict reported without the image or the time it was measured: %+v", r.Compat)
	}
	if r.Compat.Note != compatMeasuredNote {
		t.Fatalf("measured row carried the unverified note: %q", r.Compat.Note)
	}
	// spec_validation is a different axis with a different source and must not be
	// overwritten by the measurement block.
	if r.Compat.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q, want passed", r.Compat.SpecValidation)
	}
}

// ?agent= is the DISC-002 Agent dimension, live since 0022. An unknown value has
// to be a 400 rather than a silently ignored filter, for the same reason the
// unavailable dimensions are: a shared URL must never come back as a full page
// that looks filtered.
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
