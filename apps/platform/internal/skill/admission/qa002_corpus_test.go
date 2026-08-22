package ingest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// QA-002: the deliberately broken half of the spec-validation corpus.
//
// The 45 pinned-commit seed packages are the legal samples and prove only that
// validation does not reject good input. This test drives the other half — one
// mutated package per failure mode — through the same two calls the real import
// path makes (skillpkg.PackageFS then skillpkg.Validate) and compares every finding
// against a curated expectation file.
//
// Env-gated and skipped by default, the same shape as the cross-workspace tests
// that need SKILLHUB_TEST_DATABASE_URL: the corpus is produced by a script that
// downloads three pinned repo archives, so it cannot be built inside a CI job
// that has no network budget for it.
//
//	python tools/qa/skillpkg-corpus/generate.py --out <dir>
//	QA002_CORPUS=<dir> go test ./internal/skill/admission -run QA002 -v
//
// Expectations are asserted, not recorded: error and warning codes must match
// exactly, and info codes are a subset check because disclosure noise (external
// URLs, dependency lists) varies with each base package's content and is not
// what a blocking decision is made of.
type qa002Expectation struct {
	Base string `json:"base"`
	// ArchiveError is set when skillpkg.PackageFS is expected to refuse the archive
	// outright, which happens before any finding can be produced.
	ArchiveError bool     `json:"archive_error"`
	Blocked      bool     `json:"blocked"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
	InfoIncludes []string `json:"info_includes"`
	Breaks       string   `json:"breaks"`
	Note         string   `json:"note"`
}

type qa002File struct {
	Variants map[string]qa002Expectation `json:"variants"`
}

// Five levels: internal/skill/admission is one deeper than the internal/ingest
// this path was written for, and the 2026-08-20 boundary reshuffle (ADR-038,
// ADR-040) moved the package without moving the count. The test is env-gated,
// so it read four levels up and found nothing without anyone noticing.
const qa002ExpectedPath = "../../../../../tools/qa/skillpkg-corpus/expected-findings.json"

func TestQA002BrokenPackageCorpus(t *testing.T) {
	dir := os.Getenv("QA002_CORPUS")
	if dir == "" {
		t.Skip("set QA002_CORPUS to the directory tools/qa/skillpkg-corpus/generate.py wrote")
	}

	raw, err := os.ReadFile(qa002ExpectedPath)
	if err != nil {
		t.Fatalf("read expectations: %v", err)
	}
	var want qa002File
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse expectations: %v", err)
	}

	zips, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	if err != nil || len(zips) == 0 {
		t.Fatalf("no zips in %s (err %v) — run generate.py first", dir, err)
	}

	seen := map[string]bool{}
	for _, path := range zips {
		id := strings.TrimSuffix(filepath.Base(path), ".zip")
		seen[id] = true
		exp, ok := want.Variants[id]
		if !ok {
			t.Errorf("%s: corpus has no expectation; add it to %s", id, qa002ExpectedPath)
			continue
		}
		t.Run(id, func(t *testing.T) { checkQA002Variant(t, path, exp) })
	}
	for id := range want.Variants {
		if !seen[id] {
			t.Errorf("%s: expected but not produced by generate.py", id)
		}
	}
}

func checkQA002Variant(t *testing.T, path string, exp qa002Expectation) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	fsys, err := skillpkg.PackageFS(data)
	switch {
	case exp.ArchiveError && err == nil:
		t.Fatalf("archive was accepted; expected it to be refused before validation")
	case exp.ArchiveError:
		if !errors.Is(err, skillpkg.ErrBadArchive) {
			t.Fatalf("archive refused with %v; expected skillpkg.ErrBadArchive", err)
		}
		return
	case err != nil:
		t.Fatalf("archive refused (%v); expected it to reach validation", err)
	}

	report := skillpkg.Validate(fsys)
	if report.Blocked != exp.Blocked {
		t.Errorf("blocked = %v, want %v", report.Blocked, exp.Blocked)
	}

	got := qa002CodesBySeverity(report)
	assertSetEqual(t, "error", got[skillpkg.SeverityError], exp.Errors)
	assertSetEqual(t, "warning", got[skillpkg.SeverityWarning], exp.Warnings)
	assertSubset(t, "info", got[skillpkg.SeverityInfo], exp.InfoIncludes)
}

// qa002CodesBySeverity collapses the report to distinct codes per severity. The
// path and message are deliberately dropped: the same code fires once per file
// in a real package, and pinning the count would make the corpus fail whenever a
// base package upstream gains a file.
func qa002CodesBySeverity(r skillpkg.Report) map[skillpkg.Severity][]string {
	sets := map[skillpkg.Severity]map[string]bool{}
	for _, f := range r.Findings {
		if sets[f.Severity] == nil {
			sets[f.Severity] = map[string]bool{}
		}
		sets[f.Severity][f.Code] = true
	}
	out := map[skillpkg.Severity][]string{}
	for sev, set := range sets {
		for code := range set {
			out[sev] = append(out[sev], code)
		}
		sort.Strings(out[sev])
	}
	return out
}

func assertSetEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s codes:\n  got  %v\n  want %v", label, got, want)
	}
}

func assertSubset(t *testing.T, label string, got, want []string) {
	t.Helper()
	have := map[string]bool{}
	for _, c := range got {
		have[c] = true
	}
	for _, c := range want {
		if !have[c] {
			t.Errorf("%s codes: missing %q (got %v)", label, c, got)
		}
	}
}
