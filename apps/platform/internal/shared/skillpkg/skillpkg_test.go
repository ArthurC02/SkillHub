package skillpkg

import (
	"fmt"
	"io/fs"
	"regexp"
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

func TestBadYAMLFindingKeepsTheParserOut(t *testing.T) {
	r := Validate(pkg("---\nname: [unclosed\n---\nbody", nil))
	for _, f := range r.Findings {
		if f.Code != "frontmatter-invalid-yaml" {
			continue
		}
		if strings.Contains(f.Message, "yaml:") || strings.Contains(f.Message, "expected") {
			t.Fatalf("the parser's English reached the finding: %q", f.Message)
		}
		return
	}
	t.Fatal("no frontmatter-invalid-yaml finding")
}

func TestAnUnknownFrontmatterFieldBlocks(t *testing.T) {
	md := "---\nname: x\ndescription: d\nauto_run: true\n---\n"
	r := Validate(pkg(md, nil))
	if codes(r)["frontmatter-unknown-field"] != SeverityError {
		t.Fatalf("want an error: %+v", r.Findings)
	}
	if !r.Blocked {
		t.Fatal("an unknown field must block; the reference validator rejects it and so do some clients")
	}

	r = Validate(pkg("---\nname: x\ndescription: d\nmetadata:\n  team: platform\n---\n", nil))
	if codes(r)["frontmatter-unknown-field"] != "" {
		t.Fatalf("metadata is a specification field: %+v", r.Findings)
	}
}

func TestWarningsDoNotBlock(t *testing.T) {
	md := "---\nname: x\ndescription: d\n---\nSee [ref](docs/gone.md).\n"
	r := Validate(pkg(md, nil))
	c := codes(r)
	if c["license-unknown"] != SeverityWarning {
		t.Fatalf("want license-unknown warning: %+v", r.Findings)
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
	urls := urlFindings(r)
	if len(urls) != 1 || len(urls[0].Details) != 1 || !strings.Contains(urls[0].Details[0], "notes.md") {
		t.Fatalf("want 1 deduped url finding referencing notes.md, got %+v", urls)
	}
	if r.Blocked {
		t.Fatal("disclosures must not block")
	}
}

func urlFindings(r Report) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Code == "external-url" {
			out = append(out, f)
		}
	}
	return out
}

func TestURLDisclosuresAggregateByHost(t *testing.T) {
	var refs strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&refs, "see https://schemas.example.com/ns/%d\n", i)
	}
	refs.WriteString("and https://other.example.org/a\n")
	r := Validate(pkg(goodMD, map[string]string{"reference.md": refs.String()}))

	urls := urlFindings(r)
	if len(urls) != 2 {
		t.Fatalf("want one finding per host, got %d: %+v", len(urls), urls)
	}

	big := urls[1]
	if big.Code != "external-url" || !strings.Contains(big.Message, "schemas.example.com") || !strings.Contains(big.Message, "40") {
		t.Fatalf("summary must name host and count, got %q", big.Message)
	}
	if strings.Count(big.Message, "https://") > urlExamplesPerHost {
		t.Fatalf("summary must name at most %d examples, got %q", urlExamplesPerHost, big.Message)
	}
	if len(big.Details) != 40 {
		t.Fatalf("aggregation must keep every reference, got %d", len(big.Details))
	}
	if !strings.HasPrefix(big.Details[0], "reference.md: ") {
		t.Fatalf("details must keep the referencing file, got %q", big.Details[0])
	}
}

func TestManifestLimitsCountRunesNotBytes(t *testing.T) {
	desc := strings.Repeat("繁", 777)
	r := Validate(pkg("---\nname: x\ndescription: "+desc+"\nlicense: MIT\n---\n", nil))
	if r.Blocked {
		t.Fatalf("a 777-character description is within the 1024-character limit: %+v", r.Findings)
	}

	r = Validate(pkg("---\nname: x\ndescription: "+strings.Repeat("繁", 1025)+"\n---\n", nil))
	if codes(r)["description-too-long"] != SeverityError {
		t.Fatalf("1025 characters must still be rejected: %+v", r.Findings)
	}

	r = Validate(pkg("---\nname: "+strings.Repeat("繁", 65)+"\ndescription: d\n---\n", nil))
	if codes(r)["name-too-long"] != SeverityError {
		t.Fatalf("65-character name must be rejected: %+v", r.Findings)
	}
}

func TestLicenseFallsBackToPackageLicenseFile(t *testing.T) {
	const mit = "MIT License\n\nCopyright (c) 2026 Someone\n\n" +
		"Permission is hereby granted, free of charge, to any person obtaining a copy\n"
	noLicenseMD := "---\nname: x\ndescription: d\n---\n"

	r := Validate(pkg(noLicenseMD, map[string]string{"LICENSE": mit}))
	if r.LicenseExpression != "MIT" || r.LicenseSource != licenseSourcePackageFile {
		t.Fatalf("want MIT from the package file, got %q/%q", r.LicenseExpression, r.LicenseSource)
	}
	c := codes(r)
	if _, unknown := c["license-unknown"]; unknown {
		t.Fatalf("a recognised LICENSE file is not an unknown license: %+v", r.Findings)
	}
	if c["license-from-package-file"] != SeverityInfo {
		t.Fatalf("file-derived license must be disclosed as such: %+v", r.Findings)
	}

	r = Validate(pkg(goodMD, map[string]string{"LICENSE": "Apache License\nVersion 2.0\n"}))
	if r.LicenseExpression != "MIT" || r.LicenseSource != licenseSourceManifest {
		t.Fatalf("manifest declaration must win, got %q/%q", r.LicenseExpression, r.LicenseSource)
	}

	for name, files := range map[string]map[string]string{
		"no license file":   nil,
		"unrecognised text": {"LICENSE": "All rights reserved. Ask us nicely."},
	} {
		r = Validate(pkg(noLicenseMD, files))
		if r.LicenseExpression != "" || codes(r)["license-unknown"] != SeverityWarning {
			t.Fatalf("%s: want unknown license warning, got %q %+v", name, r.LicenseExpression, r.Findings)
		}
	}
}

const (
	mitText    = "MIT License\n\nCopyright (c) 2026 Someone\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\n"
	apacheText = "Apache License\nVersion 2.0, January 2004\n"
	iscText    = "Permission to use, copy, modify, and/or distribute this software for any purpose\n"
)

func TestLicenseProvenancePrecedence(t *testing.T) {
	noLicenseMD := "---\nname: x\ndescription: d\n---\n"

	for _, tc := range []struct {
		name       string
		skillMD    string
		files      map[string]string
		wantSPDX   string
		wantSource string
	}{{
		name:    "manifest beats both files",
		skillMD: goodMD,
		files: map[string]string{
			"LICENSE":          apacheText,
			CarriedLicenseFile: iscText,
		},
		wantSPDX: "MIT", wantSource: licenseSourceManifest,
	}, {
		name:    "package file beats the carried repo file",
		skillMD: noLicenseMD,
		files: map[string]string{
			"LICENSE":          apacheText,
			CarriedLicenseFile: mitText,
		},
		wantSPDX: "Apache-2.0", wantSource: licenseSourcePackageFile,
	}, {
		name:     "carried repo file is used when the package states nothing",
		skillMD:  noLicenseMD,
		files:    map[string]string{CarriedLicenseFile: mitText},
		wantSPDX: "MIT", wantSource: licenseSourceRepoFile,
	}, {

		name:     "license filename matching is case-insensitive",
		skillMD:  noLicenseMD,
		files:    map[string]string{"license": mitText},
		wantSPDX: "MIT", wantSource: licenseSourcePackageFile,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			r := Validate(pkg(tc.skillMD, tc.files))
			if r.LicenseExpression != tc.wantSPDX || r.LicenseSource != tc.wantSource {
				t.Fatalf("got %q/%q, want %q/%q",
					r.LicenseExpression, r.LicenseSource, tc.wantSPDX, tc.wantSource)
			}
			if _, unknown := codes(r)["license-unknown"]; unknown {
				t.Fatalf("resolved license must not also warn unknown: %+v", r.Findings)
			}
		})
	}

	r := Validate(pkg(noLicenseMD, map[string]string{CarriedLicenseFile: mitText}))
	if codes(r)["license-from-repo-file"] != SeverityInfo {
		t.Fatalf("carried repo license must be disclosed as such: %+v", r.Findings)
	}

	r = Validate(pkg(noLicenseMD, map[string]string{
		"LICENSE":          "All rights reserved. Ask us nicely.",
		CarriedLicenseFile: mitText,
	}))
	if r.LicenseExpression != "" || codes(r)["license-unknown"] != SeverityWarning {
		t.Fatalf("unrecognised package license must stay unknown, got %q/%q: %+v",
			r.LicenseExpression, r.LicenseSource, r.Findings)
	}
}

func TestLicenseManifestPointerResolvesReferencedFile(t *testing.T) {

	const seedPointer = "Complete terms in LICENSE.txt"

	for _, name := range []string{"brand-guidelines", "internal-comms"} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg(
				"---\nname: "+name+"\ndescription: d\nlicense: "+seedPointer+"\n---\n",
				map[string]string{"LICENSE.txt": apacheText},
			))
			if r.LicenseExpression != "Apache-2.0" || r.LicenseSource != licenseSourceManifestRef {
				t.Fatalf("got %q/%q, want Apache-2.0/%s",
					r.LicenseExpression, r.LicenseSource, licenseSourceManifestRef)
			}
			if codes(r)["license-from-manifest-reference"] != SeverityInfo {
				t.Fatalf("resolving a pointer must be disclosed: %+v", r.Findings)
			}
		})
	}

	r := Validate(pkg(
		"---\nname: x\ndescription: d\nlicense: SEE LICENSE IN LICENSE.txt\n---\n",
		map[string]string{"LICENSE.txt": apacheText},
	))
	if r.LicenseExpression != "Apache-2.0" || r.LicenseSource != licenseSourceManifestRef {
		t.Fatalf("npm spelling: got %q/%q", r.LicenseExpression, r.LicenseSource)
	}

	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"target missing", nil},
		{"target unrecognised", map[string]string{"LICENSE.txt": "All rights reserved."}},
		{"a different license file is not the named one", map[string]string{"LICENSE": mitText}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Validate(pkg("---\nname: x\ndescription: d\nlicense: "+seedPointer+"\n---\n", tc.files))
			if r.LicenseExpression != seedPointer || r.LicenseSource != licenseSourceManifest {
				t.Fatalf("got %q/%q, want the string verbatim under %s",
					r.LicenseExpression, r.LicenseSource, licenseSourceManifest)
			}
		})
	}
}

func TestLicensePointerTarget(t *testing.T) {
	for in, want := range map[string]string{
		"SEE LICENSE IN LICENSE.txt":     "LICENSE.txt",
		"see licence in COPYING":         "COPYING",
		"Complete terms in LICENSE.txt":  "LICENSE.txt",
		"Complete terms in LICENSE.txt.": "LICENSE.txt",
		"Full license text in LICENSE":   "LICENSE",
		"See the license in LICENSE.md":  "LICENSE.md",

		"MIT":                    "",
		"Apache-2.0":             "",
		"Proprietary, ask Bob":   "",
		"SEE LICENSE IN ../LICE": "",
		"see license in a/b.txt": "",
		"licensed in spirit":     "",
	} {
		got, ok := licensePointerTarget(in)
		if !ok {
			got = ""
		}
		if got != want {
			t.Errorf("licensePointerTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeSPDX(t *testing.T) {
	for in, want := range map[string]string{
		"MIT":            "MIT",
		"mit":            "MIT",
		"  Apache 2.0  ": "Apache-2.0",
		"apache-2.0":     "Apache-2.0",
		"GPLv3":          "GPL-3.0",
		"the unlicense":  "Unlicense",

		"BSD":                  "BSD",
		"Proprietary, ask Bob": "Proprietary, ask Bob",
	} {
		if got := normalizeSPDX(in); got != want {
			t.Errorf("normalizeSPDX(%q) = %q, want %q", in, got, want)
		}
	}

	r := Validate(pkg("---\nname: x\ndescription: d\nlicense: apache 2.0\n---\n", nil))
	if r.LicenseExpression != "Apache-2.0" || r.LicenseSource != licenseSourceManifest {
		t.Fatalf("got %q/%q", r.LicenseExpression, r.LicenseSource)
	}
}

func TestDetectLicense(t *testing.T) {
	cases := map[string]string{
		"Apache License\nVersion 2.0, January 2004\n":                                      "Apache-2.0",
		"Permission is hereby granted, free of charge, to any person":                      "MIT",
		"Permission to use, copy, modify, and/or distribute this software for any purpose": "ISC",
		"GNU GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007":                              "GPL-3.0",
		"GNU GENERAL PUBLIC LICENSE\nVersion 2, June 1991":                                 "GPL-2.0",
		"Redistribution and use in source and binary forms, with or without\n" +
			"modification, are permitted\n3. Neither the name of the copyright holder": "BSD-3-Clause",
		"This is free and unencumbered software released into the public domain.": "Unlicense",
		"Copyright a company. Do what you want, we guess.":                        "",
	}
	for text, want := range cases {
		if got := detectLicense([]byte(text)); got != want {
			t.Errorf("detectLicense(%.40q) = %q, want %q", text, got, want)
		}
	}
}

func TestEmbeddedCodeIsDisclosed(t *testing.T) {
	block := func(lang string, n int) string {
		return "```" + lang + "\n" + strings.Repeat("print(1)\n", n) + "```\n"
	}

	r := Validate(pkg(goodMD+block("python", 180), nil))
	f := findingByCode(r, "embedded-script")
	if f == nil || f.Severity != SeverityWarning {
		t.Fatalf("a 180-line python block must be disclosed: %+v", r.Findings)
	}
	if !strings.Contains(f.Message, "python: 180") || f.Path != "SKILL.md" {
		t.Fatalf("message must name the language and line count, got %+v", f)
	}
	if r.Blocked {
		t.Fatal("disclosure must not block")
	}

	many := goodMD
	for i := 0; i < 6; i++ {
		many += block("bash", 10)
	}
	if findingByCode(Validate(pkg(many, nil)), "embedded-script") == nil {
		t.Fatalf("60 lines across 6 blocks must be disclosed")
	}

	quiet := goodMD + block("python", 8) + "```json\n" + strings.Repeat("{}\n", 100) + "```\n"
	if f := findingByCode(Validate(pkg(quiet, nil)), "embedded-script"); f != nil {
		t.Fatalf("short snippets and non-runnable fences must not warn: %+v", f)
	}
}

func TestEmbeddedCodeBoundaryLines(t *testing.T) {
	block := func(lang string, n int) string {
		return "```" + lang + "\n" + strings.Repeat("print(1)\n", n) + "```\n"
	}

	if f := findingByCode(Validate(pkg(goodMD+block("python", maxEmbeddedBlockLines), nil)), "embedded-script"); f != nil {
		t.Fatalf("a block of exactly %d lines must not be disclosed: %+v", maxEmbeddedBlockLines, f)
	}
	if f := findingByCode(Validate(pkg(goodMD+block("python", maxEmbeddedBlockLines+1), nil)), "embedded-script"); f == nil {
		t.Fatalf("a block of %d lines must be disclosed", maxEmbeddedBlockLines+1)
	}

	atTotal := goodMD
	for i := 0; i < 5; i++ {
		atTotal += block("bash", maxEmbeddedTotalLines/5)
	}
	if f := findingByCode(Validate(pkg(atTotal, nil)), "embedded-script"); f != nil {
		t.Fatalf("blocks totalling exactly %d lines must not be disclosed: %+v", maxEmbeddedTotalLines, f)
	}

	overTotal := atTotal + block("bash", 1)
	if f := findingByCode(Validate(pkg(overTotal, nil)), "embedded-script"); f == nil {
		t.Fatalf("blocks totalling %d lines must be disclosed", maxEmbeddedTotalLines+1)
	}
}

func findingByCode(r Report, code string) *Finding {
	for i, f := range r.Findings {
		if f.Code == code {
			return &r.Findings[i]
		}
	}
	return nil
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

func details(r Report, code string) ([]string, bool) {
	for _, f := range r.Findings {
		if f.Code == code {
			return f.Details, true
		}
	}
	return nil, false
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestDependencyExtraction(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		md          string
		wantListed  []string
		wantNot     []string
		wantWarning []string
		noWarning   bool
	}{{

		name:       "install line in SKILL.md",
		md:         goodMD + "\n## Dependencies\n\n```bash\npip install pandas pycountry openpyxl\n```\n",
		wantListed: []string{"pandas", "pycountry", "openpyxl"},
		noWarning:  true,
	}, {

		name: "scripts import what nothing declares",
		files: map[string]string{
			"scripts/validate.py": "import defusedxml.ElementTree as ET\nimport lxml.etree\nimport os, sys\n",
		},
		wantListed:  []string{"defusedxml", "lxml"},
		wantNot:     []string{"os", "sys"},
		wantWarning: []string{"defusedxml", "lxml"},
	}, {
		name: "sibling and stdlib imports are not dependencies",
		files: map[string]string{
			"scripts/main.py":     "import helpers\nfrom office import validate\nimport json\nimport pandas\n",
			"scripts/helpers.py":  "import re\n",
			"scripts/office/v.py": "import io\n",
		},
		wantListed:  []string{"pandas"},
		wantNot:     []string{"helpers", "office", "json", "re", "io"},
		wantWarning: []string{"pandas"},
	}, {
		name: "import name is compared as its distribution name",
		files: map[string]string{
			"scripts/w.py": "from docx import Document\nimport dateutil.parser\n",
		},
		md:         goodMD + "\n```bash\npip install python-docx python-dateutil\n```\n",
		wantListed: []string{"python-docx", "python-dateutil"},
		wantNot:    []string{"docx", "dateutil"},
		noWarning:  true,
	}, {
		name: "a manifest is where undeclared imports belong, so no warning",
		files: map[string]string{
			"requirements.txt": "pandas>=2.0.0\nlxml==6.1.1\n",
			"scripts/a.py":     "import pandas\nimport chardet\n",
		},
		wantListed: []string{"pandas", "lxml", "chardet"},
		noWarning:  true,
	}, {
		name: "node builtins and relative specifiers are not dependencies",
		files: map[string]string{
			"build.mjs": `import fs from "node:fs";
import path from "path";
import { load } from "./local.mjs";
import pptx from "pptxgenjs";
import { x } from "@scope/pkg/sub";
`,
		},
		wantListed:  []string{"pptxgenjs", "@scope/pkg"},
		wantNot:     []string{"node:fs", "path", "./local.mjs"},
		wantWarning: []string{"pptxgenjs"},
	}, {
		name:      "a prompt-only package with no dependencies says nothing",
		md:        goodMD,
		noWarning: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := tc.md
			if md == "" {
				md = goodMD
			}
			r := Validate(pkg(md, tc.files))
			info, hasInfo := details(r, "package-dependencies")
			warn, hasWarn := details(r, "undeclared-dependency")

			if len(tc.wantListed) == 0 && hasInfo {
				t.Fatalf("want no dependency finding, got %v", info)
			}
			for _, want := range tc.wantListed {
				if !contains(info, want) {
					t.Errorf("package-dependencies missing %q: %v", want, info)
				}
			}
			for _, bad := range tc.wantNot {
				if contains(info, bad) || contains(warn, bad) {
					t.Errorf("%q must not be reported as a dependency: %v / %v", bad, info, warn)
				}
			}
			if tc.noWarning && hasWarn {
				t.Fatalf("want no undeclared-dependency warning, got %v", warn)
			}
			for _, want := range tc.wantWarning {
				if !contains(warn, want) {
					t.Errorf("undeclared-dependency missing %q: %v", want, warn)
				}
			}

			if r.Blocked {
				t.Errorf("dependency findings must never block: %+v", r.Findings)
			}
		})
	}
}

func TestASymlinkEntryIsBlockedInsteadOfBeingReadAsAFile(t *testing.T) {
	m := pkg(goodMD, nil)
	m["reference/host-passwd"] = &fstest.MapFile{Data: []byte("/etc/passwd"), Mode: fs.ModeSymlink}
	m["scripts/run.sh"] = &fstest.MapFile{Data: []byte("SKILL.md"), Mode: fs.ModeSymlink}

	r := Validate(m)
	if sev := codes(r)["symlink-entry"]; sev != SeverityError {
		t.Fatalf("want symlink-entry as an error, got %+v", r.Findings)
	}
	if !r.Blocked {
		t.Error("a link accepted by admission cannot be extracted by the runtime")
	}
	if codes(r)["script-file"] != "" {
		t.Error("a link named .sh is not a script the package ships; its body is a path")
	}
	var msg string
	for _, f := range r.Findings {
		if f.Code == "symlink-entry" && f.Path == "reference/host-passwd" {
			msg = f.Message
		}
	}
	if !strings.Contains(msg, "/etc/passwd") {
		t.Errorf("the message must name where the link points, got %q", msg)
	}
}

func TestNonRegularEntriesAreBlockedBeforeRuntime(t *testing.T) {
	m := pkg(goodMD, nil)
	m["devices/console"] = &fstest.MapFile{Mode: fs.ModeDevice}
	m["pipes/input.sh"] = &fstest.MapFile{Mode: fs.ModeNamedPipe}

	r := Validate(m)
	if sev := codes(r)[CodeUnsupportedEntryType]; sev != SeverityError || !r.Blocked {
		t.Fatalf("non-regular entries must block admission: %+v", r.Findings)
	}
	if codes(r)[CodeScriptFile] != "" {
		t.Fatal("a named pipe with a .sh suffix is not a script file")
	}
}

func TestArchiveFindingsSurviveAPackageWithNoSkillMD(t *testing.T) {
	f, ok := ArchiveEntryFinding("../../evil.sh")
	if !ok || f.Severity != SeverityError {
		t.Fatalf("a traversal entry must be an error-level finding, got %+v / %v", f, ok)
	}
	r := Validate(archiveFS{FS: pkg("", nil), findings: []Finding{f}})
	if !r.Blocked || codes(r)[CodeEntryPathEscape] != SeverityError {
		t.Fatalf("want the archive finding and skill-md-missing, got %+v", r.Findings)
	}
	if codes(r)["skill-md-missing"] != SeverityError {
		t.Fatalf("the tree's own findings must still be produced: %+v", r.Findings)
	}
}

func TestEntryNamesThatAreNotPathsInThePackage(t *testing.T) {
	for _, name := range []string{
		"../../evil.sh", "..", "a/../../b", "nested/..", `..\..\evil.sh`,
		"/etc/cron.d/evil", `C:\Windows\evil.bat`, "c:/windows/evil.bat",
	} {
		if _, ok := ArchiveEntryFinding(name); !ok {
			t.Errorf("%q was accepted as a path inside the package", name)
		}
	}

	for _, name := range []string{
		"SKILL.md", "reference/..hidden.md", "a..b/c.md", "scripts/run.sh", "dir/",
	} {
		if f, ok := ArchiveEntryFinding(name); ok {
			t.Errorf("%q was refused: %s", name, f.Message)
		}
	}
}

type archiveFS struct {
	fs.FS
	findings []Finding
}

func (a archiveFS) ArchiveFindings() []Finding { return a.findings }

func TestCategorizeOrdersFindingsTheSameWayEveryRun(t *testing.T) {
	const twoWarnings = `---
name: two-warnings
description: Has two non-string metadata values.
license: MIT
metadata:
  version: 1
  updated: 2026-01-01
---

# Two Warnings
`
	want := lines(Validate(pkg(twoWarnings, nil)).Categorize().Warnings)
	if len(want) != 2 {
		t.Fatalf("fixture must produce exactly two warnings, got %v", want)
	}
	for i := range 50 {
		got := lines(Validate(pkg(twoWarnings, nil)).Categorize().Warnings)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("run %d ordered the warnings differently:\ngot  %v\nwant %v", i, got, want)
		}
	}
}

func lines(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Code+"|"+f.Path+"|"+f.Message)
	}
	return out
}

func TestSecretsAreFoundInOversizedAndBinaryFiles(t *testing.T) {
	const key = "AKIAIOSFODNN7EXAMPLE"
	for _, tc := range []struct {
		name, path, data string
	}{
		{"over the scan cap", "fixtures/dump.sql", "-- aws_access_key_id = " + key + "\n" +
			strings.Repeat("x", maxScanBytes+1)},
		{"binary", "bin/tool", "\x00\x01\x02ELF" + key + "\x00\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Validate(pkg(goodMD, map[string]string{tc.path: tc.data}))
			if codes(r)[CodePossibleSecret] != SeverityError {
				t.Fatalf("the credential in %s was not found: %+v", tc.path, r.Findings)
			}
			if !r.Blocked {
				t.Fatal("a package carrying a credential must not be importable")
			}
		})
	}
}

func TestOversizedFileSaysHowMuchWasScanned(t *testing.T) {
	r := Validate(pkg(goodMD, map[string]string{"big.txt": strings.Repeat("x", maxScanBytes+1)}))
	for _, f := range r.Findings {
		if f.Code == "file-not-scanned" {
			if !strings.Contains(f.Message, fmt.Sprint(maxScanBytes)) {
				t.Fatalf("the disclosure does not say what was read: %q", f.Message)
			}
			return
		}
	}
	t.Fatalf("want file-not-scanned info: %+v", r.Findings)
}

func TestUnlabelledFencedCodeIsDisclosed(t *testing.T) {
	untagged := "```\n" + strings.Repeat("import os\nos.system('rm -rf /')\n", 90) + "```\n"

	r := Validate(pkg(goodMD+untagged, nil))
	f := findingByCode(r, CodeUnlabelledCodeBlock)
	if f == nil || f.Severity != SeverityInfo || f.Path != "SKILL.md" {
		t.Fatalf("a 180-line untagged fence must be disclosed: %+v", r.Findings)
	}
	if !strings.Contains(f.Message, "180") {
		t.Errorf("the disclosure must carry the line count, got %q", f.Message)
	}
	if r.Blocked {
		t.Error("a disclosure must not block")
	}

	if findingByCode(r, CodeEmbeddedScript) != nil {
		t.Error("an untagged block must not be counted as embedded script")
	}

	short := goodMD + "```\n" + strings.Repeat("$ ls\n", 5) + "```\n"
	if f := findingByCode(Validate(pkg(short, nil)), CodeUnlabelledCodeBlock); f != nil {
		t.Errorf("a 5-line untagged fence must stay quiet: %+v", f)
	}
}

func TestEveryDisclosureCodeIsDistinctAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range DisclosureCodes {
		if c == "" {
			t.Fatal("empty disclosure code")
		}
		if seen[c] {
			t.Errorf("duplicate disclosure code %q", c)
		}
		seen[c] = true
	}
	if len(DisclosureCodes) == 0 {
		t.Fatal("DisclosureCodes is empty; the catalogue assertion would pass vacuously")
	}
}

var cjkRune = regexp.MustCompile(`\p{Han}`)

func TestFindingMessagesAreTraditionalChinese(t *testing.T) {
	r := Validate(pkg("---\nname: BadName!\n---\n", nil))
	c := r.Categorize()
	all := append(append([]Finding{}, c.Errors...), c.Warnings...)
	all = append(all, c.Infos...)
	if len(all) == 0 {
		t.Fatal("fixture must produce at least one finding")
	}
	for _, f := range all {
		if !cjkRune.MatchString(f.Message) {
			t.Errorf("%s: message has no Traditional Chinese sentence, got %q", f.Code, f.Message)
		}
	}
}
