package skillpkg

import (
	"fmt"
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

// import-report.md §4 Top-3: one seed package emitted 321 external-url findings,
// which made the whole info layer unreadable. One line per host, full list kept.
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
	// Sorted by host: other.example.org before schemas.example.com.
	big := urls[1]
	if !strings.Contains(big.Message, "40 external URL(s) on schemas.example.com") {
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

// import-report.md §6.1 bug 1: len() is bytes, so a Traditional Chinese
// description hit the "1024 characters" limit at 341 characters.
func TestManifestLimitsCountRunesNotBytes(t *testing.T) {
	desc := strings.Repeat("繁", 777) // 777 runes, 2331 bytes
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

// import-report.md §4 Top-1: 37 of 45 seed packages declared no license in
// frontmatter while their repository plainly stated one.
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

	// The manifest still wins, and is labelled as the manifest.
	r = Validate(pkg(goodMD, map[string]string{"LICENSE": "Apache License\nVersion 2.0\n"}))
	if r.LicenseExpression != "MIT" || r.LicenseSource != licenseSourceManifest {
		t.Fatalf("manifest declaration must win, got %q/%q", r.LicenseExpression, r.LicenseSource)
	}

	// Neither, or an unrecognised text, stays unknown — never guessed.
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

// ADR-021: three provenance tiers, each weaker evidence about *this* package
// than the one above it, and each one stops the search.
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
		skillMD: goodMD, // declares license: MIT
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
		// curated-skill-list.md §5.1 row 8: iamursky/sokrati ships a lowercase
		// `license`. The old exact-name lookup reported it as unknown.
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

	// A carried repo-level license is disclosed as covering the repository, not
	// this package — the §5.3 trap is a repo-root MIT over content the repo
	// author never owned.
	r := Validate(pkg(noLicenseMD, map[string]string{CarriedLicenseFile: mitText}))
	if codes(r)["license-from-repo-file"] != SeverityInfo {
		t.Fatalf("carried repo license must be disclosed as such: %+v", r.Findings)
	}

	// No fall-through: an unreadable package-local license is not answered by the
	// repository's, which would be the §5.3 misattribution.
	r = Validate(pkg(noLicenseMD, map[string]string{
		"LICENSE":          "All rights reserved. Ask us nicely.",
		CarriedLicenseFile: mitText,
	}))
	if r.LicenseExpression != "" || codes(r)["license-unknown"] != SeverityWarning {
		t.Fatalf("unrecognised package license must stay unknown, got %q/%q: %+v",
			r.LicenseExpression, r.LicenseSource, r.Findings)
	}
}

// ADR-021 待決策 #1: a frontmatter `license` that names a file is a pointer, not
// a declaration. `anthropics/skills` ships `Complete terms in LICENSE.txt` on
// brand-guidelines and internal-comms, and recording that verbatim lost the
// Apache-2.0 sitting in the file right beside it.
func TestLicenseManifestPointerResolvesReferencedFile(t *testing.T) {
	// The exact string the two seed packages carry.
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

	// npm's own spelling resolves identically.
	r := Validate(pkg(
		"---\nname: x\ndescription: d\nlicense: SEE LICENSE IN LICENSE.txt\n---\n",
		map[string]string{"LICENSE.txt": apacheText},
	))
	if r.LicenseExpression != "Apache-2.0" || r.LicenseSource != licenseSourceManifestRef {
		t.Fatalf("npm spelling: got %q/%q", r.LicenseExpression, r.LicenseSource)
	}

	// Every way of failing to resolve keeps the old behaviour: the author's
	// string, recorded verbatim under the manifest tier. Never a guess, and never
	// answered by some other file that happens to be present.
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
		// Real declarations must never be mistaken for pointers, and a pointer
		// that leaves the package root is not evidence about the package.
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
		// Ambiguous or unknown strings are reported as the author wrote them,
		// never guessed into an SPDX id (DISC-003).
		"BSD":                  "BSD",
		"Proprietary, ask Bob": "Proprietary, ask Bob",
	} {
		if got := normalizeSPDX(in); got != want {
			t.Errorf("normalizeSPDX(%q) = %q, want %q", in, got, want)
		}
	}

	// Normalisation applies to the manifest tier, which is the only free text.
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

// import-report.md §4 Top-2: five seed packages carried ~180 lines of Python
// inside SKILL.md and the extension-based scan reported no scripts at all.
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

	// Many small blocks add up past the total threshold even though no single
	// block crosses the per-block one.
	many := goodMD
	for i := 0; i < 6; i++ {
		many += block("bash", 10)
	}
	if findingByCode(Validate(pkg(many, nil)), "embedded-script") == nil {
		t.Fatalf("60 lines across 6 blocks must be disclosed")
	}

	// A usage snippet, and a long block of something that does not run, stay
	// quiet: a finding on every SKILL.md is a finding nobody reads.
	quiet := goodMD + block("python", 8) + "```json\n" + strings.Repeat("{}\n", 100) + "```\n"
	if f := findingByCode(Validate(pkg(quiet, nil)), "embedded-script"); f != nil {
		t.Fatalf("short snippets and non-runnable fences must not warn: %+v", f)
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
