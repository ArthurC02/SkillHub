package skillpkg_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// The measurement ADR-044 decision 4 made the unlock condition for escalating
// frontmatter-unknown-field from warning to error.
//
// The decision was taken without it: escalating retroactively blocks catalogue
// content, and how much content was not knowable offline. This counts it — how
// many of the 45 pinned-commit seed packages carry a field outside the six the
// specification defines, and which fields.
//
// Env-gated like the QA-002 corpus, and for the same reason: the packages come
// from pinned repo archives that have to be downloaded.
//
//	python tools/content/import_seed.py --pack-only <dir>
//	SEED_CORPUS=<dir> go test ./internal/shared/skillpkg -run Census -v
//
// It asserts nothing about the distribution. A census that failed the build
// when the number moved would be a policy, and the policy is ADR-044's to make.
// The one thing it does assert is that every package parsed: a census over a
// corpus that silently failed to load is a zero that means nothing.
func TestSpecFrontmatterCensus(t *testing.T) {
	dir := os.Getenv("SEED_CORPUS")
	if dir == "" {
		t.Skip("set SEED_CORPUS to the directory import_seed.py --pack-only wrote")
	}
	zips, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	if err != nil || len(zips) == 0 {
		t.Fatalf("no zips in %s (err %v) — run import_seed.py --pack-only first", dir, err)
	}

	byField := map[string][]string{}
	var carriers []string
	for _, path := range zips {
		name := filepath.Base(path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fsys, err := skillpkg.PackageFS(data)
		if err != nil {
			t.Fatalf("%s: the corpus package did not open (%v)", name, err)
		}
		report := skillpkg.Validate(fsys)
		if report.Manifest == nil {
			t.Fatalf("%s: no frontmatter parsed; this package cannot be counted", name)
		}
		// Extra is not the same set as "unknown": metadata is a specification
		// field with no typed home, so it is parked there too. The six are what
		// escalation would keep accepting, so the six come out.
		unknown := false
		for k := range report.Manifest.Extra {
			if slices.Contains(skillpkg.SpecFields, k) {
				continue
			}
			unknown = true
			byField[k] = append(byField[k], name)
		}
		if unknown {
			carriers = append(carriers, name)
		}
	}

	fields := make([]string, 0, len(byField))
	for k := range byField {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	sort.Strings(carriers)

	t.Logf("packages: %d; carrying a field outside the six: %d", len(zips), len(carriers))
	for _, f := range fields {
		sort.Strings(byField[f])
		t.Logf("  %-24s %2d  %v", f, len(byField[f]), byField[f])
	}
	if len(carriers) > 0 {
		t.Logf("escalating to error would block on import: %v", carriers)
	}
}
