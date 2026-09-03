package catalog

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
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
	// The disclosure list, not five booleans (04 丙-29 ④). Codes rather than
	// labels: the wording is allowed to be edited, the identity is not.
	for _, want := range []string{"script-file", "embedded-script", "external-url"} {
		if !hasDisclosure(r.Disclosures, want) {
			t.Errorf("%s missing from disclosures: %+v", want, r.Disclosures)
		}
	}
	// Every entry carries its own words, so no surface has to keep a table.
	for _, d := range r.Disclosures {
		if d.Label == "" || d.Note == "" {
			t.Errorf("disclosure %q has no words: %+v", d.Code, d)
		}
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
	s := &Service{} // no store configured
	if _, ok := s.scanPackage(t.Context(), "packages/missing.zip"); ok {
		t.Fatal("scanPackage reported success with no object store")
	}
}

func TestOwnerReadsFailClosedWhenCallbacksAreMissing(t *testing.T) {
	if _, _, err := (&Service{}).CatalogSkill(t.Context(), pgtype.UUID{}); err == nil {
		t.Fatal("CatalogSkill succeeded without Registry's owner read")
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
	got := licenseFrom(VersionFacts{LicenseExpression: &expr, LicenseSource: &src})
	if got.Expression != "MIT" || got.Source != "repo-license-file" {
		t.Fatalf("license = %+v, want MIT from repo-license-file", got)
	}
	if got.SourceNote == "" {
		t.Error("repo-level license carries no note explaining what it covers")
	}
	if got.Status.Value != string(LicenseStatusDeclared) {
		t.Errorf("status = %q, want declared; nothing records a reviewer's confirmation", got.Status.Value)
	}

	unknown := licenseFrom(VersionFacts{})
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

	got := sourceFrom(SourceFacts{
		SourceType:       "git",
		SourceURL:        &url,
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
	available := sourceFrom(SourceFacts{
		SourceType:    "git",
		SourceURL:     &url,
		LastCheckedAt: pgtype.Timestamptz{Time: checked, Valid: true},
	})
	if available.LastCheckedAt == "" || available.UnavailableSince != "" {
		t.Errorf("available source = %+v, want a check time and no outage", available)
	}
	never := sourceFrom(SourceFacts{SourceType: "upload", ContentHash: "sha256:abc"})
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

func hasDisclosure(list []disclosure, code string) bool {
	for _, d := range list {
		if d.Code == code {
			return true
		}
	}
	return false
}

// A measured verdict has to say what was measured, and the rule has to hold for
// the good news as well as the bad.
//
// The defect this locks down: `activated` shipped with an empty note while
// `not_activated` and `transpiled` each carried a careful caveat. So the value
// a reader most wants to believe was the one qualified least — and the thing it
// was not saying is that the measurement used prompts that name the skill
// (02:CONTENT-007), because the platform's own spike put autonomous triggering
// at 0 (PDM-011). 「已啟用」 was being read as an answer to a question nobody had
// asked the sandbox.
//
// `unverified` is exempt on both axes: there is no measurement to qualify, and
// the block note already says so — repeating it per axis is checklist 第 14 條.
func TestAMeasuredCompatibilityVerdictSaysWhatWasMeasured(t *testing.T) {
	for name, words := range map[string]axisWords{
		"capability": capabilityWords,
		"runtime":    runtimeWords,
	} {
		for value := range words {
			if value == "unverified" {
				continue
			}
			if axis(words, value).Note == "" {
				t.Errorf("%s/%s has no note: a measured verdict that does not say "+
					"under what conditions is read as a broader claim than it is", name, value)
			}
		}
	}
}

// 02:GEN-002, in as many words: a generated package must not be shown as an
// unknown source, because its origin IS recorded — it just is not a URL.
//
// Without the switch arm added for it, sourceFrom falls through to
// SourceTrustUnknown and the detail page tells the owner "沒有可查核的來源紀錄"
// about a package the platform wrote from words that same owner typed, while the
// row holding those words sits one join away. Nothing fails; the sentence is just
// false.
func TestAGeneratedSourceIsNotAnUnknownSource(t *testing.T) {
	task := "我每個月要把廠商寄來的掃描單據整理成一份表格交出去。"
	model, prompt := "gpt-5.4-mini", "generate-skill/v1"

	got := sourceFrom(SourceFacts{
		SourceType:             "generated",
		ContentHash:            "sha256:abc",
		TaskDescription:        &task,
		GeneratorModel:         &model,
		GeneratorPromptVersion: &prompt,
	})

	if got.Trust.Value == string(SourceTrustUnknown) {
		t.Error("a generated package was reported as an unknown source")
	}
	if got.Trust.Value != string(SourceTrustGenerated) {
		t.Errorf("trust = %q, want %q", got.Trust.Value, SourceTrustGenerated)
	}
	// The three that make the record reproduce the package (ADR-047 決策 1).
	if got.TaskDescription != task || got.GeneratorModel != model || got.GeneratorPromptVersion != prompt {
		t.Errorf("provenance lost in serialisation: %+v", got)
	}
	// It is not a rung above traceable on the same ladder, and the copy must not
	// read like one: there is nothing upstream to trace to, and nobody has
	// reviewed anything.
	if got.Trust.Value == string(SourceTrustTraceable) || got.Trust.Value == string(SourceTrustManuallyConfirmed) {
		t.Error("generated borrowed a trust level that means somebody checked something")
	}

	// The other three source types must not start carrying it.
	upload := sourceFrom(SourceFacts{SourceType: "upload", ContentHash: "sha256:abc"})
	if upload.Trust.Value != string(SourceTrustUnknown) || upload.TaskDescription != "" {
		t.Errorf("upload picked up the generation record: %+v", upload)
	}
}
