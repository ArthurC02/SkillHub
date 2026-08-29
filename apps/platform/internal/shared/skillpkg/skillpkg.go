// Package skillpkg parses and statically validates Agent Skills packages
// (INGEST-003, INGEST-006..008; SKILL-002). It is pure analysis over an fs.FS:
// nothing in the package is ever executed (iron rule 1, ADR-007), and the same
// code serves uploads (zip), git imports (directory), and tests (fstest.MapFS).
package skillpkg

import (
	"fmt"
	"io"
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
	Name        string `json:"name"`
	Description string `json:"description"`
	License     string `json:"license,omitempty"`
	// Compatibility is the specification's environment-requirements field
	// ("Designed for Claude Code", "Requires Python 3.14+ and uv"). One of the
	// six the specification defines, and the one this parser was missing.
	Compatibility string         `json:"compatibility,omitempty"`
	AllowedTools  []string       `json:"allowed_tools,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// SpecRevision identifies the Agent Skills specification this validator was
// written against, and it is a pair of hashes rather than a version because the
// specification has neither a version string nor a tag nor a release
// (contracts/spec/SOURCE.json, ADR-044).
//
// It is a constant so that every sentence the platform says about conformance
// can name it. Before 2026-08-22 the produced INSTALL.md told every downloader
// the package was "valid against the Agent Skills specification" while no such
// document existed anywhere in this repository — the sentence's only real
// meaning was "it passed the function below", which is not what a reader takes
// from it. A claim about an external standard has to be able to say which one.
const SpecRevision = "agentskills.io, agentskills/agentskills@217be54 (2026-08-04)"

// SpecFields is the complete frontmatter vocabulary of that revision. Six, and
// the specification's reference validator rejects anything outside it.
var SpecFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

// Report is the full validation outcome. Findings are grouped by severity at
// the presentation layer; Blocked is true when any error-level finding exists.
// A passing report means "format valid", never "safe" or "effective" (ADR-007).
type Report struct {
	Manifest *Manifest `json:"manifest"`
	// LicenseExpression is the license the package actually evidences, and
	// LicenseSource says which artefact it came from — one of the ADR-021
	// provenance tiers (licenseSourceManifest, licenseSourceManifestRef,
	// licenseSourcePackageFile, licenseSourceRepoFile). Empty means unknown,
	// which is the only case that may be treated as unknown (DISC-003).
	LicenseExpression string    `json:"license_expression,omitempty"`
	LicenseSource     string    `json:"license_source,omitempty"`
	Findings          []Finding `json:"findings"`
	Blocked           bool      `json:"blocked"`
}

func (r *Report) add(sev Severity, code, path, msg string) {
	r.addFinding(Finding{Severity: sev, Code: code, Path: path, Message: msg})
}

func (r *Report) addFinding(f Finding) {
	r.Findings = append(r.Findings, f)
	if f.Severity == SeverityError {
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

// Categorize splits r.Findings by severity, each bucket ordered by
// (Code, Path, Message).
//
// The sort is not cosmetic. Findings are raised while walking Go maps —
// frontmatter fields, metadata entries — so two validations of the same bytes
// append them in different orders. Everything that has to be reproducible flows
// through here: skillhub-manifest.json carries these lists, and content_hash is
// taken over the zip that carries the manifest, so an unsorted bucket made two
// packagings of one input produce two different content_hash values and defeat
// the reuse lookup that is supposed to stop a second set of bytes ever existing
// (ADR-027 decisions 1 and 2).
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
	for _, bucket := range [][]Finding{c.Errors, c.Warnings, c.Infos} {
		sort.SliceStable(bucket, func(i, j int) bool {
			a, b := bucket[i], bucket[j]
			if a.Code != b.Code {
				return a.Code < b.Code
			}
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			return a.Message < b.Message
		})
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
	// maxCompatibilityLen is the specification's "1-500 characters if provided".
	maxCompatibilityLen = 500
	// maxScanBytes bounds content scanning per file.
	// ponytail: flat cap, oversized files are disclosed and skipped; stream
	// scanning only if real packages ever exceed it.
	maxScanBytes = 1 << 20 // 1 MiB
)

// ArchiveSource is implemented by an fs.FS that was opened from an archive and
// can still say what that archive declared, before the fs view normalised it.
//
// It exists because archive/zip's fs.FS rewrites entry names: `../../evil.sh` is
// presented as `evil.sh` and `/etc/cron.d/evil` as `etc/cron.d/evil`. A check
// written against the tree therefore cannot see a traversal entry at all — by
// the time anything walks, the entry has silently changed identity and the file
// list a reviewer approves is not the one the archive carries (04 丙-15 D-1/D-2).
// Only the opener sees the raw names, so it hands its findings in here rather
// than every caller having to remember to ask for them separately.
type ArchiveSource interface {
	ArchiveFindings() []Finding
}

// The content-disclosure codes: what a package was found to *contain*, as
// opposed to whether it validates. They are the vocabulary the search row and
// the detail view turn into words, which is why they are constants and exported
// — the surface that renders them used to keep its own hand-written list, and
// what a hand-written list does is fall behind the scanner. It fell behind by
// five: symlink-entry, undeclared-dependency, file-not-scanned,
// package-dependencies and entry-path-escape were all being found and none of
// them had a word (04 丙-29 ④, 稽核 01).
//
// [DisclosureCodes] below is the whole set, and catalog asserts against it.
// Adding a code here without a catalogue entry turns that test red, which is the
// only reason this list exists as a list.
//
// Deliberately NOT here: the spec, frontmatter and licence verdicts
// (`name-missing`, `license-unknown`, `file-ref-missing`, …). Those say whether
// the package is well formed, not what is inside it, and they have their own
// surfaces. Widening this set to "every code" would put "frontmatter has no
// closing ---" on a risk badge.
const (
	// CodeEntryPathEscape is one code for both shapes of the same rule: an entry
	// name that is not a path inside the package. Same severity, same remedy —
	// the message carries which shape it was.
	CodeEntryPathEscape = "entry-path-escape"
	// CodePossibleSecret is the one blocking code that reads file *content*
	// rather than package structure. Exported since before the rest because the
	// generation path has to tell it apart from the other eleven: those are
	// formatting slips a second attempt at the same prompt usually fixes, and a
	// model writing a credential-shaped line is a writing habit that reproduces
	// (ADR-048). A string literal in the other package would have made that
	// distinction silently wrong the day this one was renamed.
	CodePossibleSecret = "possible-secret"
	// CodeScriptFile: the package carries a script file. Never executed here.
	CodeScriptFile = "script-file"
	// CodeEmbeddedScript: runnable code written into SKILL.md itself, which no
	// file list can show (SKILL-003, CONTENT-006).
	CodeEmbeddedScript = "embedded-script"
	// CodeUnlabelledCodeBlock: a long fenced block with no language tag. Not
	// embedded-script, because nothing here can tell what it is — which is the
	// disclosure. Dropping five characters used to make a block invisible to
	// both the size check and dependency extraction.
	CodeUnlabelledCodeBlock = "unlabelled-code-block"
	// CodeExternalURL: package content points at addresses outside the platform.
	CodeExternalURL = "external-url"
	// CodeBinaryFile: a non-text file, whose contents static scanning cannot read.
	CodeBinaryFile = "binary-file"
	// CodeDependencyFile: the package ships a dependency manifest.
	CodeDependencyFile = "dependency-file"
	// CodePackageDependencies: the dependencies the scan could name.
	CodePackageDependencies = "package-dependencies"
	// CodeUndeclaredDependency: imported but declared nowhere in the package.
	CodeUndeclaredDependency = "undeclared-dependency"
	// CodeSymlinkEntry: an entry that is a link rather than a file.
	CodeSymlinkEntry = "symlink-entry"
	// CodeFileNotScanned: a file over maxScanBytes, of which only the first
	// megabyte was read. The secret scan therefore did not see the rest.
	CodeFileNotScanned = "file-not-scanned"
)

// DisclosureCodes is every content-disclosure code, so a renderer can assert it
// has words for all of them instead of discovering a gap from a support ticket.
// Order is the declaration order above and carries no meaning; the display order
// belongs to whoever renders them.
var DisclosureCodes = []string{
	CodeEntryPathEscape, CodePossibleSecret, CodeScriptFile, CodeEmbeddedScript,
	CodeUnlabelledCodeBlock, CodeExternalURL, CodeBinaryFile, CodeDependencyFile,
	CodePackageDependencies, CodeUndeclaredDependency, CodeSymlinkEntry,
	CodeFileNotScanned,
}

// ArchiveEntryFinding checks one raw archive entry name — the name as the
// archive declares it — and returns the finding to record when that name is not
// a path the package could contain.
//
// Error level, because this is an attempt and not format noise. A package cannot
// contain `../../evil.sh` or `/etc/cron.d/evil`; an entry that says it does is
// aimed at whatever unpacks it. Nothing here is a zip-slip — the platform never
// writes the tree to disk — but the archive and the fs view answer to two
// different names, and the one a human reviews must be the one the package
// declares.
//
// Backslashes are folded to slashes first: a zip entry name is defined to use
// forward slashes, so a backslash in one is either a literal character or an
// entry written for a Windows extractor that will treat it as a separator, and
// `..\..\evil.sh` has to be caught either way. packaging.travels() makes the same
// two judgements on the way out; this is the way in.
func ArchiveEntryFinding(name string) (Finding, bool) {
	clean := strings.ReplaceAll(name, `\`, "/")
	switch {
	case strings.HasPrefix(clean, "/"), hasDriveLetter(clean):
		return Finding{Severity: SeverityError, Code: CodeEntryPathEscape, Path: name,
			Message: "archive entry declares an absolute path, which is not a location inside the package; " +
				"the extracted tree would not be the tree this archive declares"}, true
	case clean == "..", strings.HasPrefix(clean, "../"),
		strings.Contains(clean, "/../"), strings.HasSuffix(clean, "/.."):
		return Finding{Severity: SeverityError, Code: CodeEntryPathEscape, Path: name,
			Message: "archive entry walks out of the package with ..; " +
				"the extracted tree would not be the tree this archive declares"}, true
	}
	return Finding{}, false
}

func hasDriveLetter(path string) bool {
	return len(path) >= 2 && path[1] == ':' &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}

// Validate parses SKILL.md and statically checks the package rooted at fsys.
func Validate(fsys fs.FS) Report {
	var r Report

	// First, so an archive-level finding survives the early returns below: a
	// package with no SKILL.md at all still declared those entries.
	if a, ok := fsys.(ArchiveSource); ok {
		for _, f := range a.ArchiveFindings() {
			r.addFinding(f)
		}
	}

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
		case "compatibility":
			// A specification field, and one this parser did not know until
			// 2026-08-22 — so a Skill that correctly declared its environment
			// requirements was told it had an unknown key. ADR-044.
			m.Compatibility, _ = v.(string)
		case "allowed-tools":
			switch t := v.(type) {
			case string:
				// The specification says space-separated (`Bash(git:*) Bash(jq:*) Read`).
				// Commas are accepted too — this parser read comma-separated
				// before the specification was pinned, and refusing a comma now
				// would reject stored packages over punctuation nobody documented
				// as significant.
				for _, s := range strings.Fields(strings.ReplaceAll(t, ",", " ")) {
					if s = strings.TrimSpace(s); s != "" {
						m.AllowedTools = append(m.AllowedTools, s)
					}
				}
			case []any:
				// Not the specification's shape — a YAML list here is Claude
				// Code's extension. Parsed rather than dropped (dropping it would
				// silently widen what the Skill may do), and reported so the
				// author knows it does not travel.
				for _, e := range t {
					if s, ok := e.(string); ok {
						m.AllowedTools = append(m.AllowedTools, s)
					}
				}
				r.add(SeverityWarning, "spec-allowed-tools-not-a-string", "SKILL.md",
					"allowed-tools is a YAML list; the Agent Skills specification defines it as a "+
						"space-separated string, and a list is a client extension other runtimes may ignore")
			}
		case "metadata":
			m.Extra[k] = v
			// The specification is specific: "a map from string keys to string
			// values". Anything else is dropped by at least one client rather
			// than carried, so a Skill relying on it loses data silently.
			if mm, ok := v.(map[string]any); ok {
				for mk, mv := range mm {
					if _, isString := mv.(string); !isString {
						r.add(SeverityWarning, "spec-metadata-not-string-map", "SKILL.md",
							fmt.Sprintf("metadata.%s is not a string; the specification defines metadata as a map from string keys to string values", mk))
					}
				}
			} else if v != nil {
				r.add(SeverityWarning, "spec-metadata-not-string-map", "SKILL.md",
					"metadata is not a mapping; the specification defines it as a map from string keys to string values")
			}
		default:
			m.Extra[k] = v
			r.add(SeverityError, "frontmatter-unknown-field", "SKILL.md",
				fmt.Sprintf("unknown frontmatter field %q — the Agent Skills specification defines exactly six "+
					"(name, description, license, compatibility, metadata, allowed-tools), and its reference "+
					"validator rejects anything else, as do the upload paths of at least one major client. "+
					"Move it under metadata (a map of string keys to string values) or remove it; a "+
					"package carrying it is not something this platform can call specification-valid", k))
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
	// Blank, not just absent. The specification says "Non-empty" and its
	// reference validator strips before testing — and this is the field an agent
	// reads to decide whether to load the Skill at all, so a description of
	// spaces is a Skill nothing will ever pick up. That makes it an error rather
	// than a warning: it is not a style preference, it is a Skill that cannot work.
	case strings.TrimSpace(m.Description) == "":
		r.add(SeverityError, "description-missing", "SKILL.md", "frontmatter field description is required and must not be blank")
	case utf8.RuneCountInString(m.Description) > maxDescriptionLen:
		r.add(SeverityError, "description-too-long", "SKILL.md", fmt.Sprintf("description exceeds %d characters", maxDescriptionLen))
	}
	// Specification: "Must be 1-500 characters if provided". Counted in runes,
	// for the reason import-report.md §6.1 bug 1 records about description.
	if n := utf8.RuneCountInString(m.Compatibility); n > maxCompatibilityLen {
		r.add(SeverityWarning, "spec-compatibility-too-long", "SKILL.md",
			fmt.Sprintf("compatibility is %d characters, over the specification's %d", n, maxCompatibilityLen))
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
	licenseSourceManifestRef = "manifest-referenced-file"
	licenseSourcePackageFile = "package-license-file"
	licenseSourceRepoFile    = "repo-license-file"
)

// licensePointer matches a frontmatter `license` value that names a file instead
// of declaring a license. npm has the established convention for this
// (`"SEE LICENSE IN <filename>"`, the file required at package top level), and
// `anthropics/skills` writes the same idea as `Complete terms in LICENSE.txt`.
// Recording either verbatim as an SPDX expression loses the fact the package
// plainly states (ADR-021 待決策 #1).
//
// Deliberately a closed set of phrasings rather than a heuristic: a false match
// hands the license question to some *other* file's text, so the cost of being
// wrong is a misattributed license. Anything not matched keeps the old
// behaviour and is recorded verbatim.
var licensePointer = regexp.MustCompile(
	`(?i)^(?:see|complete|full)\s+(?:the\s+)?(?:licen[sc]e|terms)(?:\s+text)?\s+in\s+(\S+?)[.,]?$`)

// licensePointerTarget returns the package-root file a pointer-style license
// value names. The target must be a bare filename at the package root, per the
// npm convention: a pointer that walks out of the package is not evidence about
// the package.
func licensePointerTarget(license string) (string, bool) {
	m := licensePointer.FindStringSubmatch(strings.Join(strings.Fields(license), " "))
	if m == nil {
		return "", false
	}
	name := strings.Trim(m[1], `"'`)
	if name == "" || strings.ContainsAny(name, `/\`) || !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

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
		// A pointer ("SEE LICENSE IN LICENSE.txt") is not a declaration. Resolve
		// the file it names; if that fails for any reason, fall through to
		// recording the author's string verbatim, exactly as before.
		if name, ok := licensePointerTarget(r.Manifest.License); ok {
			if data, err := fs.ReadFile(fsys, name); err == nil {
				if spdx := detectLicense(data); spdx != "" {
					r.LicenseExpression, r.LicenseSource = spdx, licenseSourceManifestRef
					r.add(SeverityInfo, "license-from-manifest-reference", name, fmt.Sprintf(
						"frontmatter license points at %s rather than declaring one; that file identifies %s",
						name, spdx))
					return
				}
			}
		}
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

// bareRef matches a relative path written as prose rather than as a link —
// "Run scripts/analyze.py", "See references/passes.md", or the same path inside
// a fenced command.
//
// Markdown links alone were not enough, and the seed corpus is the evidence:
// real Skills point at their own files in running text far more often than they
// link them (docs/plans/mvp/content/content-summaries.md records one whose whole
// second pass lives in `references/`, named in prose). A checker that only reads
// `[text](target)` verifies the Skills that happen to be written as documents
// and silently skips the ones written as instructions — and the second kind is
// the kind with scripts.
//
// The anchor that keeps this out of prose is not in the pattern, it is in
// referencedPaths: **the first segment must be a directory the package actually
// has**. `scripts/analyze.py` in a package with no `scripts/` is someone talking
// about another repo; in a package that has one, it is a reference.
var bareRef = regexp.MustCompile(`(?:^|[\s"'` + "`" + `(<\[])((?:[A-Za-z0-9._-]+/)+[A-Za-z0-9._-]+\.[A-Za-z0-9]{1,8})`)

// SkillMDReferences returns the package-relative paths SKILL.md points at, in
// the order they were found and without duplicates.
//
// Exported because the packager needs the same answer for a different question.
// Validation asks "is this reference satisfied?"; packaging asks "did I remove
// something this document needs?" — and those must not be allowed to disagree
// about what counts as a reference, which two implementations eventually would.
func SkillMDReferences(fsys fs.FS) []string {
	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return nil
	}
	// A throwaway report: this is the parse, not the validation. Findings raised
	// here belong to the caller that runs Validate, not to this helper.
	body := (&Report{}).parseFrontmatter(raw)
	refs, _ := referencedPaths(fsys, body)
	return refs
}

// referencedPaths extracts what SKILL.md points at, returning the in-package
// references and, separately, the ones that escape the package root.
func referencedPaths(fsys fs.FS, body string) (refs, escapes []string) {
	seen := map[string]bool{}
	add := func(target string) {
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			return
		}
		target, _, _ = strings.Cut(target, "#") // drop anchors
		clean := strings.TrimPrefix(target, "./")
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		if !fs.ValidPath(clean) {
			escapes = append(escapes, target)
			return
		}
		refs = append(refs, clean)
	}
	for _, m := range mdRef.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	for _, m := range bareRef.FindAllStringSubmatch(body, -1) {
		// The anchor. Without it every mention of `requirements.txt` in someone
		// else's repo becomes a missing file in this one.
		dir, _, _ := strings.Cut(m[1], "/")
		if info, err := fs.Stat(fsys, dir); err != nil || !info.IsDir() {
			continue
		}
		add(m[1])
	}
	return refs, escapes
}

// checkFileReferences verifies that relative paths referenced from SKILL.md
// exist in the package (SKILL-002 檔案引用).
//
// Still a warning and not an error, deliberately. A package that arrived with a
// dangling reference is the author's package and the platform's job is to say
// so, not to refuse it — 02:SKILL-002 asks for the two severities to be shown
// apart, not for this one to block. The case that *does* block is the one the
// platform caused: see BlockedFileRemoved in skill/delivery.
func (r *Report) checkFileReferences(fsys fs.FS, body string) {
	refs, escapes := referencedPaths(fsys, body)
	for _, target := range escapes {
		r.add(SeverityWarning, "file-ref-escapes-package", "SKILL.md", fmt.Sprintf("reference %q escapes the package root", target))
	}
	for _, clean := range refs {
		if _, err := fs.Stat(fsys, clean); err != nil {
			r.add(SeverityWarning, "file-ref-missing", clean, fmt.Sprintf("SKILL.md references %q which is not in the package", clean))
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
		blocks   int
		longest  int
		total    int
		byLang   = map[string]int{}
		untagged struct{ blocks, lines, longest int }
	)
	forEachFence(body, func(lang, tag, code string) {
		lines := strings.Count(code, "\n")
		if lines == 0 {
			return
		}
		// No tag at all is its own disclosure, below. Deliberately not folded into
		// the embedded-script count: that finding names the languages it found, and
		// this is exactly the case where there is no language to name.
		if tag == "" {
			untagged.blocks++
			untagged.lines += lines
			untagged.longest = max(untagged.longest, lines)
			return
		}
		if lang == "" {
			return // tagged with something nothing here runs: json, text, output …
		}
		blocks++
		total += lines
		longest = max(longest, lines)
		byLang[lang] += lines
	})
	// SKILL-003 依語言標記判定，所以刪掉五個字元本來就整段規避——連依賴抽取一起漏掉。
	// 判定不改（那要改規格），只補揭露：平台說出「這裡有一段它讀不懂的程式碼」。
	if untagged.longest > maxEmbeddedBlockLines {
		r.add(SeverityInfo, CodeUnlabelledCodeBlock, "SKILL.md", fmt.Sprintf(
			"SKILL.md contains %d unlabelled fenced block(s) totalling %d lines; longest %d lines. "+
				"With no language tag the platform cannot tell what this code is, so it is "+
				"neither counted as embedded script nor read for dependencies.",
			untagged.blocks, untagged.lines, untagged.longest))
	}
	if longest <= maxEmbeddedBlockLines && total <= maxEmbeddedTotalLines {
		return
	}

	langs := make([]string, 0, len(byLang))
	for l, n := range byLang {
		langs = append(langs, fmt.Sprintf("%s: %d", l, n))
	}
	sort.Strings(langs)
	r.add(SeverityWarning, CodeEmbeddedScript, "SKILL.md", fmt.Sprintf(
		"SKILL.md embeds %d lines of runnable code in %d code block(s) (%s); longest block %d lines. "+
			"This code is never executed during import or scan, but the package's file list does not show it.",
		total, blocks, strings.Join(langs, ", "), longest))
}

// forEachFence calls fn once per fenced code block with the canonical runnable
// language (empty when the tag names nothing this package can run), the raw tag
// as written (empty when the fence carries none), and the block's body.
//
// It opens on *every* fence, tagged or not. Until 2026-08-29 an untagged fence
// did not open a block at all, so its contents were walked as though they were
// prose: neither the size check nor dependency extraction ever saw them, and
// deleting five characters was a complete bypass of SKILL-003. The caller that
// wants only runnable blocks filters on lang; the one that discloses untagged
// blocks filters on tag.
//
// Shared by the embedded-code size check and dependency extraction: two walks
// would eventually disagree about what counts as a code block, and then the size
// warning and the dependency list would be describing different packages.
func forEachFence(body string, fn func(lang, tag, code string)) {
	var (
		lang  string // canonical runnable language, "" if none
		tag   string // the fence's raw language tag, "" if untagged
		fence string // the delimiter that opened the current block
		code  strings.Builder
		open  bool
	)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		switch {
		case !open && fenceDelimiter(trimmed) != "":
			fence = fenceDelimiter(trimmed)
			tag = fenceTag(trimmed)
			lang = runnableFences[strings.ToLower(tag)]
			code.Reset()
			open = true
		case open && strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "":
			fn(lang, tag, code.String())
			open, fence, lang, tag = false, "", "", ""
		case open:
			code.WriteString(line)
			code.WriteByte('\n')
		}
	}
}

// fenceDelimiter returns the opening fence marker of line, or "".
//
// It no longer requires a language tag: requiring one meant an untagged fence
// was not a code block at all, which is what made deleting five characters a
// complete bypass of the SKILL-003 disclosure.
func fenceDelimiter(line string) string {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, f) {
			return f
		}
	}
	return ""
}

// fenceTag is the raw language token on an opening fence line, or "".
func fenceTag(line string) string {
	if fields := strings.Fields(strings.TrimLeft(line, "`~")); len(fields) > 0 {
		return fields[0]
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
//
// It answers by name alone, so a symlink named `run.sh` is marked here while the
// scan reports it as symlink-entry instead of script-file. The two disagree in
// the cautious direction and only for a shape no real package has; deciding it
// by mode would need the entry, which a path does not carry.
func IsScriptPath(path string) bool {
	lower := strings.ToLower(path)
	i := strings.LastIndex(lower, ".")
	return i >= 0 && scriptExts[lower[i:]]
}

// scanTree walks every file: discloses scripts, executables, dependency
// manifests, and external URLs (info), and blocks on likely secrets (error).
func (r *Report) scanTree(fsys fs.FS) {
	urlsByHost := map[string][]string{}
	deps := newDepScan()
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		// A symlink before anything else: its body is a link target, not file
		// content, so scanning it as a script or as text would describe a file
		// that does not exist. A zip carries one as an ordinary entry with a mode
		// bit, and without this the scan never says the package contains a link at
		// all (04 丙-15 D-3).
		if info.Mode()&fs.ModeSymlink != 0 {
			r.add(SeverityWarning, CodeSymlinkEntry, path, symlinkMessage(fsys, path, info.Size()))
			return nil
		}
		deps.note(path)
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
			r.add(SeverityInfo, CodeScriptFile, path, "package contains a script; it is never executed during import or scan")
		case binaryExts[ext]:
			r.add(SeverityWarning, CodeBinaryFile, path, "package contains a compiled binary; contents cannot be reviewed as text")
		case dependencyFiles[base]:
			r.add(SeverityInfo, CodeDependencyFile, path, "package declares external dependencies")
		}

		// Read a bounded prefix rather than skipping an oversized file outright.
		// Skipping disclosed the file and shipped it anyway: a 1.5 MB dump
		// carrying a credential never reached secretPatterns, so possible-secret
		// never fired and packaging's sourceBlocked never saw it. Disclosure is
		// not exclusion (PACK-001 "must not be packaged" / NFR-002).
		data, err := readCapped(fsys, path, maxScanBytes)
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		if info.Size() > maxScanBytes {
			r.add(SeverityInfo, CodeFileNotScanned, path, fmt.Sprintf(
				"file exceeds the %d byte content-scan cap; only its first %d bytes were scanned",
				maxScanBytes, len(data)))
		}
		if isBinary(data) {
			// A NUL early in the file means the text analyses below would
			// describe something that is not text — but a credential is the same
			// bytes whether or not they sit next to a NUL, so the patterns still
			// run. This is the other half of the same hole: an .so or a .db with
			// an AKIA key in it used to ship byte for byte.
			r.scanSecrets(path, data)
			return nil
		}
		content := string(data)
		deps.observe(path, content)

		for _, u := range dedupe(urlPattern.FindAllString(content, -1)) {
			h := urlHost(u)
			urlsByHost[h] = append(urlsByHost[h], path+": "+u)
		}
		r.scanSecrets(path, data)
		return nil
	})
	deps.report(r)
	r.addURLDisclosures(urlsByHost)
}

// maxLinkTarget bounds what a link target may contribute to a message a user
// reads. It is untrusted package content like any other.
const maxLinkTarget = 512

// symlinkMessage describes one link entry, naming its target when the target is
// readable, short and plain text — a zip stores a symlink's target as the entry's
// body, and where it points is the whole question a reviewer has about it.
//
// Warning, not error, and the reason is the export side: packaging.travels()
// already refuses anything that is not a regular file, so a link never reaches a
// download and PACK-001's "must not be packaged" list is satisfied by exclusion.
// Blocking here would instead reject import and every Run of a package that
// carries a benign link, which is the false-block cost ADR-007 asks to weigh.
// What was missing was not a block, it was the disclosure: the exclusion used to
// be silent at both ends.
func symlinkMessage(fsys fs.FS, path string, size int64) string {
	target, err := fs.ReadLink(fsys, path)
	if err != nil && size > 0 && size <= maxLinkTarget {
		// A zip's fs view does not implement ReadLinkFS: it presents the entry as
		// an ordinary little file whose body is the target path.
		if data, rerr := fs.ReadFile(fsys, path); rerr == nil && utf8.Valid(data) {
			target, err = string(data), nil
		}
	}
	what := "a symbolic link"
	if target = strings.TrimSpace(target); err == nil && target != "" {
		if len(target) > maxLinkTarget {
			target = target[:maxLinkTarget] + "…"
		}
		what = fmt.Sprintf("a symbolic link to %q", target)
	}
	return "package entry is " + what + ", not a file; the platform never follows it and never " +
		"writes it into a package it builds, but any tool that unpacks these bytes to disk would create the link"
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
			Severity: SeverityInfo, Code: CodeExternalURL, Message: msg, Details: refs,
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

// scanSecrets runs the credential patterns over raw bytes, so it applies equally
// to text and to whatever a binary turns out to contain.
//
// The matched value is never echoed: findings end up in logs, in the manifest
// and on screen, and secrets must not (NFR-002, TRACE-001).
func (r *Report) scanSecrets(path string, data []byte) {
	for _, pat := range secretPatterns {
		if pat.Match(data) {
			r.add(SeverityError, CodePossibleSecret, path,
				"content matches a known credential pattern; remove it before importing")
			return
		}
	}
}

// readCapped reads at most limit bytes of path. The cap is the point: an entry
// declares its own size and the scan must not be talked into a larger read by
// that declaration.
func readCapped(fsys fs.FS, path string, limit int64) ([]byte, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
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
