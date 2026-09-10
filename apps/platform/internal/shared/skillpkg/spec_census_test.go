package skillpkg_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

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
