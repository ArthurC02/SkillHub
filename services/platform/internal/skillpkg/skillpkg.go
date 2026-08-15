// Package skillpkg parses and statically validates Agent Skills packages
// (INGEST-003, INGEST-006..008; SKILL-002). It is pure analysis over an fs.FS:
// nothing in the package is ever executed (iron rule 1, ADR-007), and the same
// code serves uploads (zip), git imports (directory), and tests (fstest.MapFS).
package skillpkg

import (
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type Severity string

const (
	// SeverityError blocks the import (SKILL-002 阻擋錯誤).
	SeverityError Severity = "error"
	// SeverityWarning is surfaced but does not block (可接受警告).
	SeverityWarning Severity = "warning"
	// SeverityInfo is disclosure: scripts, URLs, dependency files.
	SeverityInfo Severity = "info"
)

// Finding is one message from validation, tied to a file where possible.
// Code is stable and machine-readable; Message is for humans.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	// Details carries the full list behind an aggregated finding, so summarising
	// for readability never costs the underlying facts (INGEST-008).
	Details []string `json:"details,omitempty"`
}

// Manifest is the parsed SKILL.md frontmatter. Extra keeps unknown fields
// as-is: the spec evolves and unknown keys are a warning, not data loss.
type Manifest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	License      string         `json:"license,omitempty"`
	AllowedTools []string       `json:"allowed_tools,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// Report is the full validation outcome. Findings are grouped by severity at
// the presentation layer; Blocked is true when any error-level finding exists.
// A passing report means "format valid", never "safe" or "effective" (ADR-007).
type Report struct {
	Manifest *Manifest `json:"manifest"`
	// LicenseExpression is the license the package actually evidences, and
	// LicenseSource says which artefact it came from — one of the ADR-021
	// provenance tiers (licenseSourceManifest, licenseSourcePackageFile,
	// licenseSourceRepoFile). Empty means unknown, which is the only case that
	// may be treated as unknown (DISC-003).
	LicenseExpression string    `json:"license_expression,omitempty"`
	LicenseSource     string    `json:"license_source,omitempty"`
	Findings          []Finding `json:"findings"`
	Blocked           bool      `json:"blocked"`
}

func (r *Report) add(sev Severity, code, path, msg string) {
	r.Findings = append(r.Findings, Finding{Severity: sev, Code: code, Path: path, Message: msg})
	if sev == SeverityError {
		r.Blocked = true
	}
}

// CategorizedFindings groups a Report's findings by severity so callers can
// present blocking errors, non-blocking warnings, and informational notes as
// separate lists instead of one undifferentiated feed (INGEST-008).
type CategorizedFindings struct {
	Errors   []Finding `json:"errors"`
	Warnings []Finding `json:"warnings"`
	Infos    []Finding `json:"infos"`
}

// Categorize splits r.Findings by severity.
func (r Report) Categorize() CategorizedFindings {
	c := CategorizedFindings{Errors: []Finding{}, Warnings: []Finding{}, Infos: []Finding{}}
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityError:
			c.Errors = append(c.Errors, f)
		case SeverityWarning:
			c.Warnings = append(c.Warnings, f)
		default:
			c.Infos = append(c.Infos, f)
		}
	}
	return c
}

// nameRule is the Agent Skills name constraint: lowercase alphanumerics
// separated by single hyphens.
var nameRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	// Name and description caps are counted in runes, which is what the Agent
	// Skills spec and these error messages both say. Counting bytes made the
	// real limit depend on the author's language — a zh-Hant description hit
	// "1024 characters" at 341 of them (import-report.md §6.1 bug 1).
	maxNameLen        = 64
	maxDescriptionLen = 1024
	// maxScanBytes bounds content scanning per file.
	// ponytail: flat cap, oversized files are disclosed and skipped; stream
	// scanning only if real packages ever exceed it.
	maxScanBytes = 1 << 20 // 1 MiB
)

// Validate parses SKILL.md and statically checks the package rooted at fsys.
func Validate(fsys fs.FS) Report {
	var r Report

	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		r.add(SeverityError, "skill-md-missing", "SKILL.md", "SKILL.md not found at package root")
		return r
	}

	body := r.parseFrontmatter(raw)
	if r.Manifest != nil {
		r.checkManifest()
		r.resolveLicense(fsys)
		r.checkFileReferences(fsys, body)
		r.checkEmbeddedCode(body)
	}
	r.scanTree(fsys)
	return r
}

// parseFrontmatter extracts and decodes the YAML frontmatter, returning the
// markdown body after it.
func (r *Report) parseFrontmatter(raw []byte) (body string) {
	s := string(raw)
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(s, "---\r\n")
	}
	if !ok {
		r.add(SeverityError, "frontmatter-missing", "SKILL.md", "SKILL.md must start with YAML frontmatter (---)")
		return s
	}
	fm, body, ok := cutClosingDelimiter(rest)
	if !ok {
		r.add(SeverityError, "frontmatter-unterminated", "SKILL.md", "frontmatter has no closing --- line")
		return s
	}

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		r.add(SeverityError, "frontmatter-invalid-yaml", "SKILL.md", fmt.Sprintf("frontmatter is not valid YAML: %v", err))
		return body
	}

	m := &Manifest{Extra: map[string]any{}}
	for k, v := range fields {
		switch k {
		case "name":
			m.Name, _ = v.(string)
		case "description":
			m.Description, _ = v.(string)
		case "license":
			m.License, _ = v.(string)
		case "allowed-tools":
			switch t := v.(type) {
			case string: // spec allows a comma-separated string
				for _, s := range strings.Split(t, ",") {
					if s = strings.TrimSpace(s); s != "" {
						m.AllowedTools = append(m.AllowedTools, s)
					}
				}
			case []any:
				for _, e := range t {
					if s, ok := e.(string); ok {
						m.AllowedTools = append(m.AllowedTools, s)
					}
				}
			}
		case "metadata": // free-form per spec, kept verbatim
			m.Extra[k] = v
		default:
			m.Extra[k] = v
			r.add(SeverityWarning, "frontmatter-unknown-field", "SKILL.md", fmt.Sprintf("unknown frontmatter field %q", k))
		}
	}
	r.Manifest = m
	return body
}

// cutClosingDelimiter splits frontmatter from body at the first line that is
// exactly "---".
func cutClosingDelimiter(s string) (fm, body string, ok bool) {
	for _, sep := range []string{"\n---\n", "\n---\r\n", "\r\n---\r\n", "\r\n---\n"} {
		if fm, body, ok = strings.Cut(s, sep); ok {
			return fm, body, true
		}
	}
	// Frontmatter may end at EOF with a bare closing line.
	for _, tail := range []string{"\n---", "\r\n---"} {
		if fm, cut := strings.CutSuffix(s, tail); cut {
			return fm, "", true
		}
	}
	return "", "", false
}

func (r *Report) checkManifest() {
	m := r.Manifest
	switch {
	case m.Name == "":
		r.add(SeverityError, "name-missing", "SKILL.md", "frontmatter field name is required")
	case utf8.RuneCountInString(m.Name) > maxNameLen:
		r.add(SeverityError, "name-too-long", "SKILL.md", fmt.Sprintf("name exceeds %d characters", maxNameLen))
	case !nameRule.MatchString(m.Name):
		r.add(SeverityError, "name-invalid", "SKILL.md", "name must be lowercase letters, digits, and single hyphens")
	}
	switch {
	case m.Description == "":
		r.add(SeverityError, "description-missing", "SKILL.md", "frontmatter field description is required")
	case utf8.RuneCountInString(m.Description) > maxDescriptionLen:
		r.add(SeverityError, "description-too-long", "SKILL.md", fmt.Sprintf("description exceeds %d characters", maxDescriptionLen))
	}
}

// licenseFileNames are the package-root filenames that state the package's own
// license. Matched case-insensitively: a seed repo ships a lowercase `license`
// (curated-skill-list.md §5.1 row 8) and the old exact-name list missed it.
var licenseFileNames = map[string]bool{
	"license": true, "license.txt": true, "license.md": true,
	"licence": true, "licence.txt": true, "licence.md": true,
	"copying": true, "copying.txt": true,
}

// CarriedLicenseFile is the repository-level license file a packer carries into
// a package cut from a monorepo subdirectory (ADR-021 tier 3), next to
// CarriedProvenanceFile which records where it came from. The name is matched
// exactly, not case-insensitively: it is written by tooling, never by an author.
const (
	CarriedLicenseFile    = "LICENSE.repo"
	CarriedProvenanceFile = "LICENSE.repo.provenance.json"
)

// License provenance tiers, in the precedence order fixed by ADR-021.
const (
	licenseSourceManifest    = "manifest"
	licenseSourcePackageFile = "package-license-file"
	licenseSourceRepoFile    = "repo-license-file"
)

// licenseSignatures map a distinctive line of licence text to its SPDX id.
// Substring matching over the file's first few KiB, no dependency: these texts
// are fixed boilerplate and the ones below do not overlap. An unrecognised file
// stays unknown rather than being guessed at (DISC-003).
var licenseSignatures = []struct{ marker, spdx string }{
	{"apache license", "Apache-2.0"}, // "Version 2.0" is checked below
	{"permission to use, copy, modify, and/or distribute this software", "ISC"},
	{"permission is hereby granted, free of charge", "MIT"},
	{"gnu affero general public license", "AGPL-3.0"},
	{"gnu lesser general public license", "LGPL-3.0"},
	{"gnu general public license", "GPL-3.0"}, // version checked below
	{"mozilla public license version 2.0", "MPL-2.0"},
	{"this is free and unencumbered software released into the public domain", "Unlicense"},
	{"redistribution and use in source and binary forms", "BSD-3-Clause"}, // clause count checked below
}

// resolveLicense records the license the package evidences and which artefact it
// came from, following the ADR-021 precedence: the manifest field, then a
// license file the package states for itself, then a repository-level license
// carried in by the packer. Each tier is weaker evidence about *this* package
// than the one above it, so LicenseSource is recorded alongside the expression
// and never collapsed into it.
//
// The tiers below the manifest matter because a repository's real license is
// very often only in a file (37 of 45 seed packages declared none in
// frontmatter, import-report.md §4 Top-1), and dropping those left the catalogue
// claiming "unknown" for licenses that were plainly stated.
//
// Every tier is still only LicenseStatusDeclared, never confirmed: a repo-level
// MIT file says nothing about whether the subdirectory's content is the repo
// author's to license (curated-skill-list.md §5.3).
func (r *Report) resolveLicense(fsys fs.FS) {
	if r.Manifest.License != "" {
		r.LicenseExpression, r.LicenseSource = normalizeSPDX(r.Manifest.License), licenseSourceManifest
		return
	}
	// The first tier present decides, even when its text is unrecognised. No
	// falling through: a package that states its own license is not licensed by
	// whatever the repository says, and letting a carried repo-level MIT answer
	// for an unreadable package-local license is exactly the §5.3 failure.
	for _, c := range licenseCandidates(fsys) {
		data, err := fs.ReadFile(fsys, c.name)
		if err != nil {
			continue
		}
		spdx := detectLicense(data)
		if spdx == "" {
			// DISC-003: unknown license must be surfaced, never assumed permissive.
			r.add(SeverityWarning, "license-unknown", "SKILL.md",
				fmt.Sprintf("no license declared in frontmatter, and %s is not a recognised license text; treated as unknown", c.name))
			return
		}
		r.LicenseExpression, r.LicenseSource = spdx, c.source
		if c.source == licenseSourceRepoFile {
			r.add(SeverityInfo, "license-from-repo-file", c.name, fmt.Sprintf(
				"no license in this package; the repository-level license carried in as %s identifies %s. "+
					"It covers the repository, not necessarily this package's content.", c.name, spdx))
		} else {
			r.add(SeverityInfo, "license-from-package-file", c.name,
				fmt.Sprintf("no license in frontmatter; %s identifies the package as %s", c.name, spdx))
		}
		return
	}
	r.add(SeverityWarning, "license-unknown", "SKILL.md", "no license declared; treated as unknown")
}

// licenseCandidates lists the package-root files that may evidence a license, in
// ADR-021 precedence order. fs.ReadDir sorts by filename, so a package carrying
// several license files always resolves the same way.
func licenseCandidates(fsys fs.FS) []struct{ name, source string } {
	var out []struct{ name, source string }
	entries, _ := fs.ReadDir(fsys, ".")
	for _, e := range entries {
		if !e.IsDir() && licenseFileNames[strings.ToLower(e.Name())] {
			out = append(out, struct{ name, source string }{e.Name(), licenseSourcePackageFile})
		}
	}
	if info, err := fs.Stat(fsys, CarriedLicenseFile); err == nil && !info.IsDir() {
		out = append(out, struct{ name, source string }{CarriedLicenseFile, licenseSourceRepoFile})
	}
	return out
}

// spdxIDs are the identifiers this scanner emits. detectLicense already returns
// canonical ones; this list exists to canonicalise the *manifest* field, which
// is free text an author typed.
var spdxIDs = []string{
	"Apache-2.0", "MIT", "ISC", "BSD-2-Clause", "BSD-3-Clause",
	"GPL-2.0", "GPL-3.0", "LGPL-3.0", "AGPL-3.0", "MPL-2.0",
	"Unlicense", "CC0-1.0", "CC-BY-4.0", "CC-BY-SA-4.0",
}

// spdxAliases are non-SPDX spellings common in frontmatter. Only unambiguous
// ones: a bare "BSD" or "GPL" does not say which variant, and guessing a license
// is worse than reporting the author's own string (DISC-003).
var spdxAliases = map[string]string{
	"apache 2.0": "Apache-2.0", "apache-2": "Apache-2.0", "apache2": "Apache-2.0",
	"apache license 2.0": "Apache-2.0", "mit license": "MIT",
	"bsd-3": "BSD-3-Clause", "bsd 3-clause": "BSD-3-Clause",
	"bsd-2": "BSD-2-Clause", "bsd 2-clause": "BSD-2-Clause",
	"mpl 2.0": "MPL-2.0", "gpl-3": "GPL-3.0", "gplv3": "GPL-3.0",
	"gpl-2": "GPL-2.0", "gplv2": "GPL-2.0", "agplv3": "AGPL-3.0",
	"cc0": "CC0-1.0", "the unlicense": "Unlicense", "public domain": "Unlicense",
}

// normalizeSPDX canonicalises a declared license string so the same license
// declared three ways compares equal (DISC-004 比較, ADR-021). An unrecognised
// string is returned trimmed but otherwise verbatim — never mapped by guesswork.
func normalizeSPDX(s string) string {
	trimmed := strings.Join(strings.Fields(s), " ")
	key := strings.ToLower(trimmed)
	for _, id := range spdxIDs {
		if strings.EqualFold(id, key) {
			return id
		}
	}
	if id, ok := spdxAliases[key]; ok {
		return id
	}
	return trimmed
}

// detectLicense returns the SPDX id of a well-known license text, or "".
func detectLicense(data []byte) string {
	head := strings.ToLower(string(data[:min(len(data), 4096)]))
	norm := strings.Join(strings.Fields(head), " ") // collapse the wrapping
	for _, sig := range licenseSignatures {
		if !strings.Contains(norm, sig.marker) {
			continue
		}
		switch sig.spdx {
		case "Apache-2.0":
			if !strings.Contains(norm, "version 2.0") {
				return ""
			}
		case "GPL-3.0":
			if strings.Contains(norm, "version 2") {
				return "GPL-2.0"
			}
			if !strings.Contains(norm, "version 3") {
				return ""
			}
		case "BSD-3-Clause":
			if !strings.Contains(norm, "neither the name of") {
				return "BSD-2-Clause"
			}
		}
		return sig.spdx
	}
	return ""
}

// mdRef matches markdown links and images: [text](target) / ![alt](target).
var mdRef = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)\)`)

// checkFileReferences verifies that relative paths referenced from SKILL.md
// exist in the package (SKILL-002 檔案引用).
func (r *Report) checkFileReferences(fsys fs.FS, body string) {
	for _, m := range mdRef.FindAllStringSubmatch(body, -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		target, _, _ = strings.Cut(target, "#") // drop anchors
		clean := strings.TrimPrefix(target, "./")
		if !fs.ValidPath(clean) {
			r.add(SeverityWarning, "file-ref-escapes-package", "SKILL.md", fmt.Sprintf("reference %q escapes the package root", target))
			continue
		}
		if _, err := fs.Stat(fsys, clean); err != nil {
			r.add(SeverityWarning, "file-ref-missing", clean, fmt.Sprintf("SKILL.md references %q which is not in the package", target))
		}
	}
}

// Embedded-code thresholds. A few lines in a fenced block illustrate usage; a
// couple of hundred are a program that happens to live inside a document, and
// the seed import found 5 packages shipping ~180 lines of Python that way while
// the file-extension scan reported no scripts at all (import-report.md §4
// Top-2). Above either threshold the package is disclosed as carrying code.
//
// ponytail: two flat line counts, chosen to sit well above a usage snippet and
// well below those 180-line blocks. Tune from real appeal traffic, not from
// theory.
const (
	maxEmbeddedBlockLines = 20 // one block this long is not an example
	maxEmbeddedTotalLines = 50 // or many smaller ones adding up
)

// runnableFences are the fence languages whose content would run somewhere if
// copied out. Prose-ish tags (json, yaml, text, markdown, diff, output) are
// deliberately absent: disclosing those as code would train users to ignore
// this finding.
var runnableFences = map[string]string{
	"python": "python", "py": "python", "python3": "python",
	"bash": "bash", "sh": "bash", "shell": "bash", "zsh": "bash", "console": "bash",
	"javascript": "javascript", "js": "javascript", "node": "javascript",
	"typescript": "typescript", "ts": "typescript",
	"ruby": "ruby", "rb": "ruby", "perl": "perl", "php": "php",
	"powershell": "powershell", "ps1": "powershell", "bat": "batch", "cmd": "batch",
	"go": "go", "rust": "rust", "r": "r", "sql": "sql",
}

// checkEmbeddedCode discloses runnable code written into SKILL.md itself
// (CONTENT-006). scanTree only sees files, so a Skill whose whole implementation
// is a fenced Python block was being presented as containing no scripts.
func (r *Report) checkEmbeddedCode(body string) {
	var (
		lang    string // non-empty while inside a runnable fence
		fence   string // the delimiter that opened the current block
		lines   int
		blocks  int
		longest int
		total   int
		byLang  = map[string]int{}
	)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		open := fenceDelimiter(trimmed)
		switch {
		case fence == "" && open != "":
			fence = open
			lang = runnableFences[strings.ToLower(strings.Fields(strings.TrimLeft(trimmed, "`~"))[0])]
			lines = 0
		case fence != "" && strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "":
			if lang != "" && lines > 0 {
				blocks++
				total += lines
				longest = max(longest, lines)
				byLang[lang] += lines
			}
			fence, lang = "", ""
		case fence != "":
			lines++
		}
	}
	if longest <= maxEmbeddedBlockLines && total <= maxEmbeddedTotalLines {
		return
	}

	langs := make([]string, 0, len(byLang))
	for l, n := range byLang {
		langs = append(langs, fmt.Sprintf("%s: %d", l, n))
	}
	sort.Strings(langs)
	r.add(SeverityWarning, "embedded-script", "SKILL.md", fmt.Sprintf(
		"SKILL.md embeds %d lines of runnable code in %d code block(s) (%s); longest block %d lines. "+
			"This code is never executed during import or scan, but the package's file list does not show it.",
		total, blocks, strings.Join(langs, ", "), longest))
}

// fenceDelimiter returns the opening fence marker of line, or "".
func fenceDelimiter(line string) string {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, f) && len(strings.Fields(strings.TrimLeft(line, "`~"))) > 0 {
			return f
		}
	}
	return ""
}

var (
	scriptExts = map[string]bool{
		".sh": true, ".bash": true, ".zsh": true, ".py": true, ".js": true,
		".mjs": true, ".ts": true, ".rb": true, ".pl": true, ".ps1": true,
		".bat": true, ".cmd": true,
	}
	binaryExts = map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
		".wasm": true, ".pyc": true, ".jar": true, ".class": true,
	}
	dependencyFiles = map[string]bool{
		"requirements.txt": true, "pyproject.toml": true, "package.json": true,
		"go.mod": true, "gemfile": true, "cargo.toml": true, "pom.xml": true,
	}

	urlPattern = regexp.MustCompile(`https?://[^\s"'<>\)\]]+`)

	// secretPatterns are deliberately few and high-confidence: a false block
	// costs an appeal (ADR-007 誤報), so fuzzy entropy checks stay out.
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                       // AWS access key id
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),             // GitHub tokens
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{32,}`),                  // OpenAI-style API keys
		regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),           // Slack tokens
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),     // key material
		regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*\S{20}`), // AWS secret assignment
	}
)

// IsScriptPath reports whether path is one of the script types scanTree
// discloses. Exported so the DISC-003 file-tree view marks exactly the files the
// scan called scripts: a second extension list in the presentation layer would
// eventually disagree with this one, and the tree is where a user decides
// whether to trust the package.
func IsScriptPath(path string) bool {
	lower := strings.ToLower(path)
	i := strings.LastIndex(lower, ".")
	return i >= 0 && scriptExts[lower[i:]]
}

// scanTree walks every file: discloses scripts, executables, dependency
// manifests, and external URLs (info), and blocks on likely secrets (error).
func (r *Report) scanTree(fsys fs.FS) {
	urlsByHost := map[string][]string{}
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		lower := strings.ToLower(path)
		ext := ""
		if i := strings.LastIndex(lower, "."); i >= 0 {
			ext = lower[i:]
		}
		base := lower
		if i := strings.LastIndex(lower, "/"); i >= 0 {
			base = lower[i+1:]
		}

		switch {
		case scriptExts[ext]:
			r.add(SeverityInfo, "script-file", path, "package contains a script; it is never executed during import or scan")
		case binaryExts[ext]:
			r.add(SeverityWarning, "binary-file", path, "package contains a compiled binary; contents cannot be reviewed as text")
		case dependencyFiles[base]:
			r.add(SeverityInfo, "dependency-file", path, "package declares external dependencies")
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxScanBytes {
			r.add(SeverityInfo, "file-not-scanned", path, "file exceeds the content-scan size cap")
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil || isBinary(data) {
			return nil
		}
		content := string(data)

		for _, u := range dedupe(urlPattern.FindAllString(content, -1)) {
			h := urlHost(u)
			urlsByHost[h] = append(urlsByHost[h], path+": "+u)
		}
		for _, pat := range secretPatterns {
			if loc := pat.FindString(content); loc != "" {
				// Never echo the matched value: findings end up in logs and
				// the UI, and secrets must not (NFR-002).
				r.add(SeverityError, "possible-secret", path, "content matches a known credential pattern; remove it before importing")
				break
			}
		}
		return nil
	})
	r.addURLDisclosures(urlsByHost)
}

// urlExamplesPerHost is how many URLs the summary line names before it stops;
// the rest stay in Finding.Details.
const urlExamplesPerHost = 3

// addURLDisclosures emits one info finding per external host instead of one per
// URL. A single seed package produced 321 URL findings — mostly OOXML schema
// namespaces — which is a list nobody reads and which buried the handful of
// findings that mattered (import-report.md §4 Top-3). The host is the answer to
// "where would this Skill connect to"; every original URL is still carried in
// Details, so nothing is lost, only folded.
func (r *Report) addURLDisclosures(byHost map[string][]string) {
	hosts := make([]string, 0, len(byHost))
	for h := range byHost {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		refs := byHost[h]
		sort.Strings(refs)
		// Distinct URLs: the same schema URI cited from ten files makes for ten
		// identical examples, which is exactly the noise being folded away.
		examples := make([]string, 0, urlExamplesPerHost)
		seen := map[string]bool{}
		shown := 0
		for _, ref := range refs {
			_, u, _ := strings.Cut(ref, ": ")
			if seen[u] {
				continue
			}
			seen[u] = true
			shown++
			if len(examples) < urlExamplesPerHost {
				examples = append(examples, u)
			}
		}
		msg := fmt.Sprintf("references %d external URL(s) on %s: %s", len(refs), h, strings.Join(examples, ", "))
		if shown > len(examples) {
			msg += fmt.Sprintf(", and %d more distinct URL(s)", shown-len(examples))
		}
		r.Findings = append(r.Findings, Finding{
			Severity: SeverityInfo, Code: "external-url", Message: msg, Details: refs,
		})
	}
}

// urlHost extracts the host for grouping; unparseable URLs group under their
// own raw text rather than being dropped.
func urlHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return raw
}

func isBinary(data []byte) bool {
	n := min(len(data), 8000)
	for _, b := range data[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

func dedupe(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
