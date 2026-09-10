package skillpkg_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestGenerateSpikeCensus(t *testing.T) {
	dir := os.Getenv("GENERATED_CORPUS")
	if dir == "" {
		t.Skip("set GENERATED_CORPUS to the directory spike.py wrote")
	}
	zips, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	if err != nil || len(zips) == 0 {
		t.Fatalf("no zips in %s (err %v) — run spike.py first", dir, err)
	}
	sort.Strings(zips)

	type row struct {
		name        string
		blocked     bool
		errCodes    []string
		warnCodes   []string
		bodyRunes   int
		placeholder []string
		hasLicense  bool
		files       int
	}
	var rows []row
	blockedCount, licenseCount := 0, 0
	errHist, warnHist := map[string]int{}, map[string]int{}

	for _, path := range zips {
		name := strings.TrimSuffix(filepath.Base(path), ".zip")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fsys, err := skillpkg.PackageFS(data)
		if err != nil {
			t.Fatalf("%s: the generated package did not open (%v)", name, err)
		}
		report := skillpkg.Validate(fsys)
		r := row{name: name, blocked: report.Blocked}

		cat := report.Categorize()
		for _, f := range cat.Errors {
			r.errCodes = append(r.errCodes, f.Code)
			errHist[f.Code]++
		}
		for _, f := range cat.Warnings {
			r.warnCodes = append(r.warnCodes, f.Code)
			warnHist[f.Code]++
		}
		if report.Blocked {
			blockedCount++
		}

		if report.Manifest != nil && strings.TrimSpace(report.Manifest.License) != "" {
			r.hasLicense = true
			licenseCount++
		}
		r.bodyRunes, r.placeholder = bodyCensus(fsys)
		r.files = countFiles(fsys)
		rows = append(rows, r)
	}

	t.Logf("packages: %d   blocked: %d   passed: %d", len(rows), blockedCount, len(rows)-blockedCount)
	t.Logf("emitted a license field despite the prompt forbidding it: %d", licenseCount)
	t.Log("")
	t.Log("name        files  blocked  body   placeholders                 error codes")
	for _, r := range rows {
		t.Logf("%-11s %5d  %-7t  %5d  %-27s  %s",
			r.name, r.files, r.blocked, r.bodyRunes,
			trunc(strings.Join(r.placeholder, ","), 27),
			strings.Join(r.errCodes, ","))
	}
	t.Log("")
	t.Logf("blocking code histogram:  %s", hist(errHist))
	t.Logf("warning  code histogram:  %s", hist(warnHist))

	out, _ := json.MarshalIndent(map[string]any{
		"packages": len(rows), "blocked": blockedCount,
		"license_emitted": licenseCount,
		"error_codes":     errHist, "warning_codes": warnHist,
	}, "", "  ")
	t.Logf("summary json:\n%s", out)
}

func bodyCensus(fsys fs.FS) (int, []string) {
	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return 0, []string{"no-SKILL.md"}
	}
	text := string(raw)

	if strings.HasPrefix(text, "---") {
		if i := strings.Index(text[3:], "\n---"); i >= 0 {
			text = text[3+i+4:]
		}
	}
	return utf8.RuneCountInString(strings.TrimSpace(text)), skillpkg.PlaceholderShapes(text)
}

func countFiles(fsys fs.FS) int {
	n := 0
	_ = fs.WalkDir(fsys, ".", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func hist(m map[string]int) string {
	if len(m) == 0 {
		return "(empty)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, "  ")
}
