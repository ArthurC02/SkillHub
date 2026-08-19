package catalog

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
)

// pkgWithScriptAndEmbeddedCode is a package that carries risk on both axes the
// DISC-008 block has to show separately: a real script file, and an
// implementation written into SKILL.md where the file list cannot show it
// (SKILL-003).
func pkgWithScriptAndEmbeddedCode() fstest.MapFS {
	body := "---\nname: demo-skill\ndescription: demo\nlicense: MIT\n---\n\n```python\n"
	for i := 0; i < 60; i++ {
		body += "print('line')\n"
	}
	body += "```\n\nSee https://example.com/docs for more.\n"
	return fstest.MapFS{
		"SKILL.md":        {Data: []byte(body)},
		"scripts/run.py":  {Data: []byte("print('hi')\n")},
		"reference/x.md":  {Data: []byte("# notes\n")},
		"assets/logo.png": {Data: []byte("\x89PNG\r\n\x1a\n")},
	}
}

func TestRiskSummarySeparatesFileScriptsFromEmbeddedCode(t *testing.T) {
	r := summarizeRisk(skillpkg.Validate(pkgWithScriptAndEmbeddedCode()))

	if r.ScanStatus != "scanned" {
		t.Fatalf("scan_status = %q, want scanned", r.ScanStatus)
	}
	if !r.HasScripts {
		t.Error("has_scripts is false for a package containing scripts/run.py")
	}
	if !r.HasEmbeddedScript {
		t.Error("has_embedded_script is false for 60 lines of Python inside SKILL.md")
	}
	if !r.HasExternalURLs {
		t.Error("has_external_urls is false for a package citing example.com")
	}
	// The embedded-script disclosure is a warning and must be readable verbatim,
	// not folded into a count: the file list is exactly what fails to show it.
	found := false
	for _, f := range r.Highlights {
		if f.Code == "embedded-script" {
			found = true
		}
	}
	if !found {
		t.Errorf("embedded-script missing from highlights: %+v", r.Highlights)
	}
	// Info-level disclosures fold to counts per code, so a package citing hundreds
	// of URLs cannot bury the findings above.
	if r.InfoCounts["script-file"] != 1 {
		t.Errorf("info_counts[script-file] = %d, want 1", r.InfoCounts["script-file"])
	}
	if r.Counts.Infos == 0 || r.Counts.Warnings == 0 {
		t.Errorf("severity counts not populated: %+v", r.Counts)
	}
}

// An unreadable package must never look like a clean one (DISC-004).
func TestDefaultRiskIsUnavailableNotClean(t *testing.T) {
	h := &Handler{} // no store configured
	if _, ok := h.scanPackage(t.Context(), "packages/missing.zip"); ok {
		t.Fatal("scanPackage reported success with no object store")
	}
}

func TestFileTreeMarksScriptsAndOmitsDirectories(t *testing.T) {
	tree := fileTree(pkgWithScriptAndEmbeddedCode())

	byPath := map[string]fileEntry{}
	for _, e := range tree {
		byPath[e.Path] = e
	}
	if len(byPath) != 4 {
		t.Fatalf("tree has %d entries, want 4 files (no directories): %+v", len(byPath), tree)
	}
	if !byPath["scripts/run.py"].IsScript {
		t.Error("scripts/run.py is not marked as a script")
	}
	if byPath["reference/x.md"].IsScript {
		t.Error("reference/x.md is marked as a script")
	}
	if byPath["scripts/run.py"].Size == 0 {
		t.Error("file size is missing from the tree")
	}
}

// ADR-021: the expression and the tier it was established at travel together. A
// repo-root license is not the package declaring one for itself.
func TestLicenseKeepsProvenanceTierAndNeverConfirms(t *testing.T) {
	expr, src := "MIT", "repo-license-file"
	got := licenseFrom(gen.SkillVersion{LicenseExpression: &expr, LicenseSource: &src})
	if got.Expression != "MIT" || got.Source != "repo-license-file" {
		t.Fatalf("license = %+v, want MIT from repo-license-file", got)
	}
	if got.SourceNote == "" {
		t.Error("repo-level license carries no note explaining what it covers")
	}
	if got.Status.Value != string(LicenseStatusDeclared) {
		t.Errorf("status = %q, want declared; nothing records a reviewer's confirmation", got.Status.Value)
	}

	unknown := licenseFrom(gen.SkillVersion{})
	if unknown.Status.Value != string(LicenseStatusUnknown) || unknown.Expression != "" {
		t.Errorf("missing license = %+v, want unknown with no expression", unknown)
	}
}

// The availability probe (0013_governance) writes two separate facts and the
// detail view has to show both: DISC-003 provenance is not traceable if the
// reader cannot tell "checked, still there" from "checked, gone for two weeks".
func TestSourceSurfacesAvailabilityProbe(t *testing.T) {
	url := "https://github.com/example/skills"
	checked := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	gone := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	got := sourceFrom(gen.SkillSource{
		SourceType:       "git",
		SourceUrl:        &url,
		ContentHash:      "sha256:abc",
		LastCheckedAt:    pgtype.Timestamptz{Time: checked, Valid: true},
		UnavailableSince: pgtype.Timestamptz{Time: gone, Valid: true},
	})
	if got.LastCheckedAt != "2026-08-14T09:30:00Z" {
		t.Errorf("last_checked_at = %q", got.LastCheckedAt)
	}
	if got.UnavailableSince != "2026-08-01T12:00:00Z" {
		t.Errorf("unavailable_since = %q", got.UnavailableSince)
	}

	// A source that answered on its last probe reports the check but no outage;
	// a never-probed source reports neither, rather than an implied "available".
	available := sourceFrom(gen.SkillSource{
		SourceType:    "git",
		SourceUrl:     &url,
		LastCheckedAt: pgtype.Timestamptz{Time: checked, Valid: true},
	})
	if available.LastCheckedAt == "" || available.UnavailableSince != "" {
		t.Errorf("available source = %+v, want a check time and no outage", available)
	}
	never := sourceFrom(gen.SkillSource{SourceType: "upload", ContentHash: "sha256:abc"})
	if never.LastCheckedAt != "" || never.UnavailableSince != "" {
		t.Errorf("never-probed source = %+v, want both absent", never)
	}
}

// DISC-008 wants the three compatibility axes apart, and the two M2 ones stated
// as unverified rather than left out.
func TestSpecValidationIsSeparateFromRuntimeCompatibility(t *testing.T) {
	if got := specValidation(skillpkg.Validate(pkgWithScriptAndEmbeddedCode())); got != "passed" {
		t.Errorf("spec_validation = %q, want passed", got)
	}
	blocked := skillpkg.Validate(fstest.MapFS{"README.md": {Data: []byte("no manifest")}})
	if got := specValidation(blocked); got != "failed" {
		t.Errorf("spec_validation for a package without SKILL.md = %q, want failed", got)
	}
}
