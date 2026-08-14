package skillpkg

import (
	"strings"
	"testing"
	"testing/fstest"
)

func pkg(skillMD string, extra map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	if skillMD != "" {
		m["SKILL.md"] = &fstest.MapFile{Data: []byte(skillMD)}
	}
	for p, content := range extra {
		m[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func codes(r Report) map[string]Severity {
	out := map[string]Severity{}
	for _, f := range r.Findings {
		out[f.Code] = f.Severity
	}
	return out
}

const goodMD = `---
name: pdf-tools
description: Work with PDF files.
license: MIT
---

# PDF Tools
`

func TestValidPackage(t *testing.T) {
	r := Validate(pkg(goodMD, nil))
	if r.Blocked {
		t.Fatalf("valid package blocked: %+v", r.Findings)
	}
	if r.Manifest == nil || r.Manifest.Name != "pdf-tools" || r.Manifest.License != "MIT" {
		t.Fatalf("manifest not parsed: %+v", r.Manifest)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("unexpected findings: %+v", r.Findings)
	}
}

func TestMissingSkillMD(t *testing.T) {
	r := Validate(pkg("", map[string]string{"readme.md": "hi"}))
	if !r.Blocked || codes(r)["skill-md-missing"] != SeverityError {
		t.Fatalf("want skill-md-missing error, got %+v", r.Findings)
	}
}

func TestFrontmatterRules(t *testing.T) {
	cases := []struct {
		name, md, wantCode string
	}{
		{"no frontmatter", "# Just markdown\n", "frontmatter-missing"},
		{"unterminated", "---\nname: x\n", "frontmatter-unterminated"},
		{"bad yaml", "---\nname: [unclosed\n---\nbody", "frontmatter-invalid-yaml"},
		{"missing name", "---\ndescription: d\n---\n", "name-missing"},
		{"bad name", "---\nname: Bad_Name\ndescription: d\n---\n", "name-invalid"},
		{"long name", "---\nname: " + strings.Repeat("a", 65) + "\ndescription: d\n---\n", "name-too-long"},
		{"missing description", "---\nname: x\n---\n", "description-missing"},
		{"long description", "---\nname: x\ndescription: " + strings.Repeat("d", 1025) + "\n---\n", "description-too-long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Validate(pkg(tc.md, nil))
			if sev := codes(r)[tc.wantCode]; sev != SeverityError {
				t.Fatalf("want %s as error, findings: %+v", tc.wantCode, r.Findings)
			}
			if !r.Blocked {
				t.Fatal("spec violations must block")
			}
		})
	}
}

func TestWarningsDoNotBlock(t *testing.T) {
	md := "---\nname: x\ndescription: d\ncustom-field: y\n---\nSee [ref](docs/gone.md).\n"
	r := Validate(pkg(md, nil))
	c := codes(r)
	if c["license-unknown"] != SeverityWarning {
		t.Fatalf("want license-unknown warning: %+v", r.Findings)
	}
	if c["frontmatter-unknown-field"] != SeverityWarning {
		t.Fatalf("want unknown-field warning: %+v", r.Findings)
	}
	if c["file-ref-missing"] != SeverityWarning {
		t.Fatalf("want file-ref-missing warning: %+v", r.Findings)
	}
	if r.Blocked {
		t.Fatal("warnings must not block")
	}
}

func TestFileReferences(t *testing.T) {
	md := goodMD + "[ok](scripts/run.py) [esc](../outside.md) [web](https://x.dev/a) [anchor](#top)\n"
	r := Validate(pkg(md, map[string]string{"scripts/run.py": "print(1)"}))
	c := codes(r)
	if c["file-ref-escapes-package"] != SeverityWarning {
		t.Fatalf("want escape warning: %+v", r.Findings)
	}
	if _, bad := c["file-ref-missing"]; bad {
		t.Fatalf("existing/external/anchor refs flagged: %+v", r.Findings)
	}
	if c["script-file"] != SeverityInfo {
		t.Fatalf("want script disclosure: %+v", r.Findings)
	}
}

func TestTreeDisclosures(t *testing.T) {
	r := Validate(pkg(goodMD, map[string]string{
		"tool.exe":         string([]byte{0x4d, 0x5a, 0x00, 0x01}),
		"requirements.txt": "requests==2.31\n",
		"notes.md":         "docs at https://docs.x.dev/guide and https://docs.x.dev/guide again",
	}))
	c := codes(r)
	if c["binary-file"] != SeverityWarning {
		t.Fatalf("want binary warning: %+v", r.Findings)
	}
	if c["dependency-file"] != SeverityInfo {
		t.Fatalf("want dependency disclosure: %+v", r.Findings)
	}
	urls := 0
	for _, f := range r.Findings {
		if f.Code == "external-url" && f.Path == "notes.md" {
			urls++
		}
	}
	if urls != 1 {
		t.Fatalf("want 1 deduped url finding for notes.md, got %d: %+v", urls, r.Findings)
	}
	if r.Blocked {
		t.Fatal("disclosures must not block")
	}
}

func TestSecretsBlockWithoutEchoingValue(t *testing.T) {
	secret := "AKIA" + strings.Repeat("A", 16)
	r := Validate(pkg(goodMD, map[string]string{"config.env": "AWS_KEY=" + secret}))
	if !r.Blocked {
		t.Fatal("secret must block import")
	}
	for _, f := range r.Findings {
		if strings.Contains(f.Message, secret) {
			t.Fatalf("finding echoes the secret value: %+v", f)
		}
	}
	if codes(r)["possible-secret"] != SeverityError {
		t.Fatalf("want possible-secret error: %+v", r.Findings)
	}
}

func TestAllowedToolsBothShapes(t *testing.T) {
	list := "---\nname: x\ndescription: d\nlicense: MIT\nallowed-tools:\n  - Bash\n  - Read\n---\n"
	csv := "---\nname: x\ndescription: d\nlicense: MIT\nallowed-tools: Bash, Read\n---\n"
	for _, md := range []string{list, csv} {
		r := Validate(pkg(md, nil))
		if len(r.Manifest.AllowedTools) != 2 || r.Manifest.AllowedTools[0] != "Bash" {
			t.Fatalf("allowed-tools not parsed: %+v", r.Manifest)
		}
	}
}

func TestCategorizeSeparatesBySeverity(t *testing.T) {
	// One finding of each severity: missing license (warning), a disclosed
	// script (info), and a secret inside that same script (error).
	secret := "AKIA" + strings.Repeat("A", 16)
	md := "---\nname: x\ndescription: d\n---\n"
	r := Validate(pkg(md, map[string]string{"run.py": "key = '" + secret + "'"}))
	if !r.Blocked {
		t.Fatalf("expected the secret to block: %+v", r.Findings)
	}

	c := r.Categorize()
	for _, f := range c.Errors {
		if f.Severity != SeverityError {
			t.Fatalf("errors bucket has non-error finding: %+v", f)
		}
	}
	for _, f := range c.Warnings {
		if f.Severity != SeverityWarning {
			t.Fatalf("warnings bucket has non-warning finding: %+v", f)
		}
	}
	for _, f := range c.Infos {
		if f.Severity != SeverityInfo {
			t.Fatalf("infos bucket has non-info finding: %+v", f)
		}
	}
	if len(c.Errors) != 1 || c.Errors[0].Code != "possible-secret" {
		t.Fatalf("want exactly one possible-secret error, got %+v", c.Errors)
	}
	if len(c.Warnings) != 1 || c.Warnings[0].Code != "license-unknown" {
		t.Fatalf("want exactly one license-unknown warning, got %+v", c.Warnings)
	}
	if len(c.Infos) != 1 || c.Infos[0].Code != "script-file" {
		t.Fatalf("want exactly one script-file info, got %+v", c.Infos)
	}
	if total := len(c.Errors) + len(c.Warnings) + len(c.Infos); total != len(r.Findings) {
		t.Fatalf("categorize dropped or duplicated findings: got %d buckets, %d raw findings", total, len(r.Findings))
	}
}

func TestCategorizeNeverNil(t *testing.T) {
	// A clean package (TestValidPackage's fixture) has zero findings; the
	// buckets must still be empty slices, not nil, so JSON encodes `[]`
	// instead of `null` for API consumers.
	c := Validate(pkg(goodMD, nil)).Categorize()
	if c.Errors == nil || c.Warnings == nil || c.Infos == nil {
		t.Fatalf("categorize buckets must be non-nil empty slices: %+v", c)
	}
}

func TestOversizedFileSkipped(t *testing.T) {
	big := strings.Repeat("x", maxScanBytes+1)
	r := Validate(pkg(goodMD, map[string]string{"big.txt": big}))
	if codes(r)["file-not-scanned"] != SeverityInfo {
		t.Fatalf("want file-not-scanned info: %+v", r.Findings)
	}
}
