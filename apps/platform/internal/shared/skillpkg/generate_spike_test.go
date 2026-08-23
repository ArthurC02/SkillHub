package skillpkg_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// The measurement docs/plans/mvp/m5/README.md 的前期驗證 1 asked for, and the
// first third of `03:GEN-009`.
//
// ADR-046 決策 6 makes skillpkg.Validate the only gate a generated package
// passes through before a version is written. Eleven of the twelve blocking
// codes are structural — SKILL.md present, well-formed frontmatter, no unknown
// field, name shape and length, description present and length, no path
// escape. The twelfth, possible-secret, is not: it matches credential patterns
// against file *content* (ADR-048, and an earlier revision of this comment
// claimed all twelve were structural). This measures whether that reading
// holds against packages a model actually produced, and it deliberately
// measures the second question too: a package can clear every one of those
// checks and still be an empty shell.
//
// Env-gated like TestSpecFrontmatterCensus, and for the same reason: the
// corpus costs money to produce and does not belong in the repo.
//
//	python <scratchpad>/spike.py <dir> <cards.json>
//	GENERATED_CORPUS=<dir> go test ./internal/shared/skillpkg -run GenerateSpike -v
//
// It asserts nothing about the distribution. What it does assert is that every
// zip opened: a census over a corpus that silently failed to load is a zero
// that means nothing.
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
		// ADR-046 決策 5: the generator must not emit this field. A model that
		// emits it anyway is not a validator failure — `license` is one of the
		// six spec fields, so it passes — which is exactly why it has to be
		// counted separately here.
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

// placeholderRe is what "格式正確的空殼" looks like in practice: a heading
// followed by a slot the author was supposed to fill in.
var placeholderRe = map[string]*regexp.Regexp{
	"TODO":      regexp.MustCompile(`(?i)\bTODO\b`),
	"FIXME":     regexp.MustCompile(`(?i)\bFIXME\b`),
	"<angle>":   regexp.MustCompile(`<[a-z_ -]{3,30}>`),
	"[bracket]": regexp.MustCompile(`\[(insert|your|описание|填|placeholder)[^\]]*\]`),
	"ellipsis":  regexp.MustCompile(`(?m)^\s*(\.\.\.|…)\s*$`),
	"xxx":       regexp.MustCompile(`(?i)\bxxx+\b`),
}

// bodyCensus returns the rune count of SKILL.md after its frontmatter, and
// which placeholder shapes appear in it. The count is a proxy, not a verdict:
// it separates "there are instructions here" from "there is a shape here",
// and nothing beyond that.
func bodyCensus(fsys fs.FS) (int, []string) {
	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return 0, []string{"no-SKILL.md"}
	}
	text := string(raw)
	// Drop the frontmatter: everything up to and including the closing ---.
	if strings.HasPrefix(text, "---") {
		if i := strings.Index(text[3:], "\n---"); i >= 0 {
			text = text[3+i+4:]
		}
	}
	var found []string
	for label, re := range placeholderRe {
		if re.MatchString(text) {
			found = append(found, label)
		}
	}
	sort.Strings(found)
	return utf8.RuneCountInString(strings.TrimSpace(text)), found
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
