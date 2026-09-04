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

// ADR-044 decision 4: a warning until 2026-08-22, an error since. The unlock
// was one measurement, not a preference - TestSpecFrontmatterCensus counted 0
// of 106 packages across the 11 pinned source repos carrying a field outside
// the six, so escalation retroactively blocks nothing that exists.
func TestAnUnknownFrontmatterFieldBlocks(t *testing.T) {
	md := "---\nname: x\ndescription: d\nauto_run: true\n---\n"
	r := Validate(pkg(md, nil))
	if codes(r)["frontmatter-unknown-field"] != SeverityError {
		t.Fatalf("want an error: %+v", r.Findings)
	}
	if !r.Blocked {
		t.Fatal("an unknown field must block; the reference validator rejects it and so do some clients")
	}
	// metadata is one of the six and has no typed home, so it lands in Extra
	// alongside the unknown keys. Parking it there must not make it unknown.
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

// details returns the Details list of the first finding with the given code,
// and whether such a finding exists at all.
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

// The three cases the M2 baseline actually produced (content-baseline-report.md
// §13.4), plus the false positives that would make the warning useless.
func TestDependencyExtraction(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		md          string
		wantListed  []string // must appear in the package-dependencies info
		wantNot     []string // must appear in neither finding
		wantWarning []string // must appear in undeclared-dependency
		noWarning   bool
	}{{
		// wrangler/add-iso3166: no scripts at all, everything declared on an
		// install line in SKILL.md. The catalog recorded only pandas.
		name:       "install line in SKILL.md",
		md:         goodMD + "\n## Dependencies\n\n```bash\npip install pandas pycountry openpyxl\n```\n",
		wantListed: []string{"pandas", "pycountry", "openpyxl"},
		noWarning:  true, // the package does declare them; the transcription was short
	}, {
		// anthropic-sa/xlsx: scripts import defusedxml and lxml and the package
		// declares neither, anywhere.
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
			// Advisory only: a package that under-declares still imports.
			if r.Blocked {
				t.Errorf("dependency findings must never block: %+v", r.Findings)
			}
		})
	}
}

// 04 丙-15 D-3: a link entry used to be scanned as an ordinary little text file,
// so the report never said the package contained a link at all.
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

// 04 丙-15 D-1/D-2: findings the archive reader hands in must reach the report
// even when validation gives up on the first line.
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
	// Names that merely look alarming are not: a false block costs an appeal.
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

// --- ADR-027 decisions 1 and 2: the same content, the same list --------------

// Findings are raised while ranging over Go maps, so the order they are appended
// in is the runtime's business, not the content's. Everything that has to
// reproduce flows through Categorize: skillhub-manifest.json prints these lists,
// and content_hash is taken over the zip that carries the manifest. Unsorted,
// two packagings of one input produced two different content_hash values, the
// reuse lookup missed, and the platform wrote a second object and a second row
// for bytes it already had.
//
// Two warnings from one map (metadata.updated, metadata.version) is the smallest
// fixture that can come out either way round; the repetition is what turns a
// coin-flip into a certainty.
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

// --- NFR-002 / PACK-001: disclosure is not exclusion -------------------------

// The two files the content scan used to walk past: one over the size cap, one
// with an early NUL. Both were disclosed and both shipped, because the scan
// returned before secretPatterns ran — so packaging's sourceBlocked never saw a
// possible-secret and wrote the credential into a download byte for byte.
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

// The oversized file is still disclosed, and the disclosure now says what was
// actually read rather than implying nothing was.
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

// SKILL-003 judges by the language tag, so until 2026-08-29 the whole rule could
// be evaded by deleting five characters: an untagged fence did not open a code
// block at all, so its lines reached neither the size check nor dependency
// extraction. The verdict still keys on the tag — narrowing it is a spec
// change — but the reader is now told the block is there.
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
	// Not embedded-script: that finding names the languages it found, and here
	// there is none to name. Reporting it as one would be inventing a language.
	if findingByCode(r, CodeEmbeddedScript) != nil {
		t.Error("an untagged block must not be counted as embedded script")
	}

	// A short untagged snippet stays quiet, on the same threshold as its tagged
	// sibling: a finding on every SKILL.md is a finding nobody reads.
	short := goodMD + "```\n" + strings.Repeat("$ ls\n", 5) + "```\n"
	if f := findingByCode(Validate(pkg(short, nil)), CodeUnlabelledCodeBlock); f != nil {
		t.Errorf("a 5-line untagged fence must stay quiet: %+v", f)
	}
}

// Every code the scanner can put on a package has to be a code some surface can
// turn into words. The list is here rather than in a comment because a code
// added without an entry in catalog's disclosure catalogue is invisible on both
// screens, which is how symlink-entry, undeclared-dependency, file-not-scanned,
// package-dependencies and entry-path-escape were all being found and none of
// them said anything (稽核 01).
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

// 04 丙-152: the Findings component prints Message verbatim in role="alert" on
// the import page, the version-upload form, the packaging page and the
// generation panel — and until this test existed, nothing ever rendered a
// Report with a non-empty findings list, so the English sentences that command
// used to reach that alert region were never caught by fixture or by CI.
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
