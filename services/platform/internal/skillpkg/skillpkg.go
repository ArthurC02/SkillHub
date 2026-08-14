// Package skillpkg parses and statically validates Agent Skills packages
// (INGEST-003, INGEST-006..008; SKILL-002). It is pure analysis over an fs.FS:
// nothing in the package is ever executed (iron rule 1, ADR-007), and the same
// code serves uploads (zip), git imports (directory), and tests (fstest.MapFS).
package skillpkg

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

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
	Findings []Finding `json:"findings"`
	Blocked  bool      `json:"blocked"`
}

func (r *Report) add(sev Severity, code, path, msg string) {
	r.Findings = append(r.Findings, Finding{Severity: sev, Code: code, Path: path, Message: msg})
	if sev == SeverityError {
		r.Blocked = true
	}
}

// nameRule is the Agent Skills name constraint: lowercase alphanumerics
// separated by single hyphens.
var nameRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
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
		r.checkFileReferences(fsys, body)
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
	case len(m.Name) > maxNameLen:
		r.add(SeverityError, "name-too-long", "SKILL.md", fmt.Sprintf("name exceeds %d characters", maxNameLen))
	case !nameRule.MatchString(m.Name):
		r.add(SeverityError, "name-invalid", "SKILL.md", "name must be lowercase letters, digits, and single hyphens")
	}
	switch {
	case m.Description == "":
		r.add(SeverityError, "description-missing", "SKILL.md", "frontmatter field description is required")
	case len(m.Description) > maxDescriptionLen:
		r.add(SeverityError, "description-too-long", "SKILL.md", fmt.Sprintf("description exceeds %d characters", maxDescriptionLen))
	}
	if m.License == "" {
		// DISC-003: unknown license must be surfaced, never assumed permissive.
		r.add(SeverityWarning, "license-unknown", "SKILL.md", "no license declared; treated as unknown")
	}
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

// scanTree walks every file: discloses scripts, executables, dependency
// manifests, and external URLs (info), and blocks on likely secrets (error).
func (r *Report) scanTree(fsys fs.FS) {
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

		for _, url := range dedupe(urlPattern.FindAllString(content, -1)) {
			r.add(SeverityInfo, "external-url", path, "references external URL "+url)
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
