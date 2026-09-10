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
	SeverityError Severity = "error"

	SeverityWarning Severity = "warning"

	SeverityInfo Severity = "info"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`

	Details []string `json:"details,omitempty"`
}

type Manifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	License     string `json:"license,omitempty"`

	Compatibility string         `json:"compatibility,omitempty"`
	AllowedTools  []string       `json:"allowed_tools,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

const SpecRevision = "agentskills.io, agentskills/agentskills@217be54 (2026-08-04)"

var SpecFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

type Report struct {
	Manifest *Manifest `json:"manifest"`

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

type CategorizedFindings struct {
	Errors   []Finding `json:"errors"`
	Warnings []Finding `json:"warnings"`
	Infos    []Finding `json:"infos"`
}

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

var nameRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	maxNameLen        = 64
	maxDescriptionLen = 1024

	maxCompatibilityLen = 500

	maxScanBytes = 1 << 20
)

type ArchiveSource interface {
	ArchiveFindings() []Finding
}

const (
	CodeEntryPathEscape = "entry-path-escape"

	CodePossibleSecret = "possible-secret"

	CodeScriptFile = "script-file"

	CodeEmbeddedScript = "embedded-script"

	CodeUnlabelledCodeBlock = "unlabelled-code-block"

	CodeExternalURL = "external-url"

	CodeBinaryFile = "binary-file"

	CodeDependencyFile = "dependency-file"

	CodePackageDependencies = "package-dependencies"

	CodeUndeclaredDependency = "undeclared-dependency"

	CodeSymlinkEntry = "symlink-entry"

	CodeUnsupportedEntryType = "unsupported-entry-type"

	CodeFileNotScanned = "file-not-scanned"

	CodeNestedArchive = "nested-archive"
)

var DisclosureCodes = []string{
	CodeEntryPathEscape, CodePossibleSecret, CodeScriptFile, CodeEmbeddedScript,
	CodeUnlabelledCodeBlock, CodeExternalURL, CodeBinaryFile, CodeDependencyFile,
	CodePackageDependencies, CodeUndeclaredDependency, CodeSymlinkEntry, CodeUnsupportedEntryType,
	CodeFileNotScanned, CodeNestedArchive,
}

var archiveSuffixes = []string{".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar"}

func LooksLikeArchive(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range archiveSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func ArchiveEntryFinding(name string) (Finding, bool) {
	if strings.Contains(name, `\`) {
		return Finding{Severity: SeverityError, Code: CodeEntryPathEscape, Path: name,
			Message: "封存項目名稱使用了反斜線；ZIP 路徑必須使用正斜線，讓匯入與 Runtime 看到同一個檔名"}, true
	}
	clean := strings.ReplaceAll(name, `\`, "/")
	switch {
	case strings.HasPrefix(clean, "/"), hasDriveLetter(clean):
		return Finding{Severity: SeverityError, Code: CodeEntryPathEscape, Path: name,
			Message: "封存項目宣告了一個絕對路徑，這不是套件內的位置；" +
				"解壓縮出來的目錄樹會與這個封存檔宣告的不同"}, true
	case clean == "..", strings.HasPrefix(clean, "../"),
		strings.Contains(clean, "/../"), strings.HasSuffix(clean, "/.."):
		return Finding{Severity: SeverityError, Code: CodeEntryPathEscape, Path: name,
			Message: "封存項目用 .. 走出了套件範圍；" +
				"解壓縮出來的目錄樹會與這個封存檔宣告的不同"}, true
	}
	return Finding{}, false
}

func hasDriveLetter(path string) bool {
	return len(path) >= 2 && path[1] == ':' &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}

func Validate(fsys fs.FS) Report {
	var r Report

	if a, ok := fsys.(ArchiveSource); ok {
		for _, f := range a.ArchiveFindings() {
			r.addFinding(f)
		}
	}

	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		r.add(SeverityError, "skill-md-missing", "SKILL.md", "套件根目錄找不到 SKILL.md")
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

func (r *Report) parseFrontmatter(raw []byte) (body string) {
	s := string(raw)
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(s, "---\r\n")
	}
	if !ok {
		r.add(SeverityError, "frontmatter-missing", "SKILL.md", "SKILL.md 必須以 YAML frontmatter（---）開頭")
		return s
	}
	fm, body, ok := cutClosingDelimiter(rest)
	if !ok {
		r.add(SeverityError, "frontmatter-unterminated", "SKILL.md", "frontmatter 沒有結尾的 --- 行")
		return s
	}

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {

		r.add(SeverityError, "frontmatter-invalid-yaml", "SKILL.md", "frontmatter 不是合法的 YAML")
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

			m.Compatibility, _ = v.(string)
		case "allowed-tools":
			switch t := v.(type) {
			case string:

				for _, s := range strings.Fields(strings.ReplaceAll(t, ",", " ")) {
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
				r.add(SeverityWarning, "spec-allowed-tools-not-a-string", "SKILL.md",
					"allowed-tools 是一個 YAML 陣列；Agent Skills 規格將它定義為以空白分隔的字串，"+
						"陣列是特定客戶端的擴充寫法，其他 Runtime 可能會忽略它")
			}
		case "metadata":
			m.Extra[k] = v

			if mm, ok := v.(map[string]any); ok {
				for mk, mv := range mm {
					if _, isString := mv.(string); !isString {
						r.add(SeverityWarning, "spec-metadata-not-string-map", "SKILL.md",
							fmt.Sprintf("metadata.%s 的值不是字串；規格將 metadata 定義為「字串鍵對應字串值」的映射", mk))
					}
				}
			} else if v != nil {
				r.add(SeverityWarning, "spec-metadata-not-string-map", "SKILL.md",
					"metadata 不是一個映射；規格將它定義為「字串鍵對應字串值」的映射")
			}
		default:
			m.Extra[k] = v
			r.add(SeverityError, "frontmatter-unknown-field", "SKILL.md",
				fmt.Sprintf("不明的 frontmatter 欄位 %q — Agent Skills 規格只定義了六個欄位"+
					"（name、description、license、compatibility、metadata、allowed-tools），其參考驗證器"+
					"會拒絕其他任何欄位，至少一個主要客戶端的上傳流程也是如此。"+
					"請把它搬進 metadata（字串鍵對應字串值的映射）或移除它；"+
					"帶有此欄位的套件無法被本平台判定為符合規格", k))
		}
	}
	r.Manifest = m
	return body
}

func cutClosingDelimiter(s string) (fm, body string, ok bool) {
	for _, sep := range []string{"\n---\n", "\n---\r\n", "\r\n---\r\n", "\r\n---\n"} {
		if fm, body, ok = strings.Cut(s, sep); ok {
			return fm, body, true
		}
	}

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
		r.add(SeverityError, "name-missing", "SKILL.md", "frontmatter 欄位 name 為必填")
	case utf8.RuneCountInString(m.Name) > maxNameLen:
		r.add(SeverityError, "name-too-long", "SKILL.md", fmt.Sprintf("name 超過 %d 個字元", maxNameLen))
	case !nameRule.MatchString(m.Name):
		r.add(SeverityError, "name-invalid", "SKILL.md", "name 只能使用小寫英文字母、數字與單一連字號")
	}
	switch {

	case strings.TrimSpace(m.Description) == "":
		r.add(SeverityError, "description-missing", "SKILL.md", "frontmatter 欄位 description 為必填，且不可為空白")
	case utf8.RuneCountInString(m.Description) > maxDescriptionLen:
		r.add(SeverityError, "description-too-long", "SKILL.md", fmt.Sprintf("description 超過 %d 個字元", maxDescriptionLen))
	}

	if n := utf8.RuneCountInString(m.Compatibility); n > maxCompatibilityLen {
		r.add(SeverityWarning, "spec-compatibility-too-long", "SKILL.md",
			fmt.Sprintf("compatibility 有 %d 個字元，超過規格上限 %d", n, maxCompatibilityLen))
	}
}

var licenseFileNames = map[string]bool{
	"license": true, "license.txt": true, "license.md": true,
	"licence": true, "licence.txt": true, "licence.md": true,
	"copying": true, "copying.txt": true,
}

const (
	CarriedLicenseFile    = "LICENSE.repo"
	CarriedProvenanceFile = "LICENSE.repo.provenance.json"
)

const (
	licenseSourceManifest    = "manifest"
	licenseSourceManifestRef = "manifest-referenced-file"
	licenseSourcePackageFile = "package-license-file"
	licenseSourceRepoFile    = "repo-license-file"
)

var licensePointer = regexp.MustCompile(
	`(?i)^(?:see|complete|full)\s+(?:the\s+)?(?:licen[sc]e|terms)(?:\s+text)?\s+in\s+(\S+?)[.,]?$`)

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

var licenseSignatures = []struct{ marker, spdx string }{
	{"apache license", "Apache-2.0"},
	{"permission to use, copy, modify, and/or distribute this software", "ISC"},
	{"permission is hereby granted, free of charge", "MIT"},
	{"gnu affero general public license", "AGPL-3.0"},
	{"gnu lesser general public license", "LGPL-3.0"},
	{"gnu general public license", "GPL-3.0"},
	{"mozilla public license version 2.0", "MPL-2.0"},
	{"this is free and unencumbered software released into the public domain", "Unlicense"},
	{"redistribution and use in source and binary forms", "BSD-3-Clause"},
}

func (r *Report) resolveLicense(fsys fs.FS) {
	if r.Manifest.License != "" {

		if name, ok := licensePointerTarget(r.Manifest.License); ok {
			if data, err := fs.ReadFile(fsys, name); err == nil {
				if spdx := detectLicense(data); spdx != "" {
					r.LicenseExpression, r.LicenseSource = spdx, licenseSourceManifestRef
					r.add(SeverityInfo, "license-from-manifest-reference", name, fmt.Sprintf(
						"frontmatter 的 license 欄位指向 %s 而非直接宣告授權；該檔案標明的授權是 %s",
						name, spdx))
					return
				}
			}
		}
		r.LicenseExpression, r.LicenseSource = normalizeSPDX(r.Manifest.License), licenseSourceManifest
		return
	}

	for _, c := range licenseCandidates(fsys) {
		data, err := fs.ReadFile(fsys, c.name)
		if err != nil {
			continue
		}
		spdx := detectLicense(data)
		if spdx == "" {

			r.add(SeverityWarning, "license-unknown", "SKILL.md",
				fmt.Sprintf("frontmatter 未宣告授權，且 %s 不是可辨識的授權條款文字；視為未知授權", c.name))
			return
		}
		r.LicenseExpression, r.LicenseSource = spdx, c.source
		if c.source == licenseSourceRepoFile {
			r.add(SeverityInfo, "license-from-repo-file", c.name, fmt.Sprintf(
				"此套件本身沒有授權宣告；隨附的 repository 層級授權檔 %s 標明的授權是 %s。"+
					"它涵蓋的是整個 repository，不必然涵蓋這個套件的內容。", c.name, spdx))
		} else {
			r.add(SeverityInfo, "license-from-package-file", c.name,
				fmt.Sprintf("frontmatter 未宣告授權；%s 標明此套件的授權是 %s", c.name, spdx))
		}
		return
	}
	r.add(SeverityWarning, "license-unknown", "SKILL.md", "未宣告授權；視為未知授權")
}

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

var spdxIDs = []string{
	"Apache-2.0", "MIT", "ISC", "BSD-2-Clause", "BSD-3-Clause",
	"GPL-2.0", "GPL-3.0", "LGPL-3.0", "AGPL-3.0", "MPL-2.0",
	"Unlicense", "CC0-1.0", "CC-BY-4.0", "CC-BY-SA-4.0",
}

var spdxAliases = map[string]string{
	"apache 2.0": "Apache-2.0", "apache-2": "Apache-2.0", "apache2": "Apache-2.0",
	"apache license 2.0": "Apache-2.0", "mit license": "MIT",
	"bsd-3": "BSD-3-Clause", "bsd 3-clause": "BSD-3-Clause",
	"bsd-2": "BSD-2-Clause", "bsd 2-clause": "BSD-2-Clause",
	"mpl 2.0": "MPL-2.0", "gpl-3": "GPL-3.0", "gplv3": "GPL-3.0",
	"gpl-2": "GPL-2.0", "gplv2": "GPL-2.0", "agplv3": "AGPL-3.0",
	"cc0": "CC0-1.0", "the unlicense": "Unlicense", "public domain": "Unlicense",
}

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

func detectLicense(data []byte) string {
	head := strings.ToLower(string(data[:min(len(data), 4096)]))
	norm := strings.Join(strings.Fields(head), " ")
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

var mdRef = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)\)`)

var bareRef = regexp.MustCompile(`(?:^|[\s"'` + "`" + `(<\[])((?:[A-Za-z0-9._-]+/)+[A-Za-z0-9._-]+\.[A-Za-z0-9]{1,8})`)

func SkillMDReferences(fsys fs.FS) []string {
	raw, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return nil
	}

	body := (&Report{}).parseFrontmatter(raw)
	refs, _ := referencedPaths(fsys, body)
	return refs
}

func referencedPaths(fsys fs.FS, body string) (refs, escapes []string) {
	seen := map[string]bool{}
	add := func(target string) {
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			return
		}
		target, _, _ = strings.Cut(target, "#")
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

		dir, _, _ := strings.Cut(m[1], "/")
		if info, err := fs.Stat(fsys, dir); err != nil || !info.IsDir() {
			continue
		}
		add(m[1])
	}
	return refs, escapes
}

func (r *Report) checkFileReferences(fsys fs.FS, body string) {
	refs, escapes := referencedPaths(fsys, body)
	for _, target := range escapes {
		r.add(SeverityWarning, "file-ref-escapes-package", "SKILL.md", fmt.Sprintf("引用 %q 逃出了套件根目錄", target))
	}
	for _, clean := range refs {
		if _, err := fs.Stat(fsys, clean); err != nil {
			r.add(SeverityWarning, "file-ref-missing", clean, fmt.Sprintf("SKILL.md 引用了 %q，但套件裡沒有這個檔案", clean))
		}
	}
}

const (
	maxEmbeddedBlockLines = 20
	maxEmbeddedTotalLines = 50
)

var runnableFences = map[string]string{
	"python": "python", "py": "python", "python3": "python",
	"bash": "bash", "sh": "bash", "shell": "bash", "zsh": "bash", "console": "bash",
	"javascript": "javascript", "js": "javascript", "node": "javascript",
	"typescript": "typescript", "ts": "typescript",
	"ruby": "ruby", "rb": "ruby", "perl": "perl", "php": "php",
	"powershell": "powershell", "ps1": "powershell", "bat": "batch", "cmd": "batch",
	"go": "go", "rust": "rust", "r": "r", "sql": "sql",
}

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

		if tag == "" {
			untagged.blocks++
			untagged.lines += lines
			untagged.longest = max(untagged.longest, lines)
			return
		}
		if lang == "" {
			return
		}
		blocks++
		total += lines
		longest = max(longest, lines)
		byLang[lang] += lines
	})

	if untagged.longest > maxEmbeddedBlockLines {
		r.add(SeverityInfo, CodeUnlabelledCodeBlock, "SKILL.md", fmt.Sprintf(
			"SKILL.md 內有 %d 個未標記語言的程式碼區塊，合計 %d 行；最長 %d 行。"+
				"沒有語言標記，平台無法判斷這是什麼程式碼，因此既不計入內嵌程式碼，也不會被讀取分析依賴套件。",
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
		"SKILL.md 內嵌了 %d 行可執行程式碼，共 %d 個程式碼區塊（%s）；最長區塊 %d 行。"+
			"這段程式碼在匯入或掃描期間不會被執行，但套件的檔案清單不會顯示它。",
		total, blocks, strings.Join(langs, ", "), longest))
}

// forEachFence walks body line by line, tracking whether it is inside a fence
// and which delimiter opened it, and calls fn with the accumulated body once
// the matching closing line is found.
func forEachFence(body string, fn func(lang, tag, code string)) {
	var (
		lang  string
		tag   string
		fence string
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

func fenceDelimiter(line string) string {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, f) {
			return f
		}
	}
	return ""
}

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

	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{32,}`),
		regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*\S{20}`),
	}
)

func IsScriptPath(path string) bool {
	lower := strings.ToLower(path)
	i := strings.LastIndex(lower, ".")
	return i >= 0 && scriptExts[lower[i:]]
}

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

		if info.Mode()&fs.ModeSymlink != 0 {
			r.add(SeverityError, CodeSymlinkEntry, path, symlinkMessage(fsys, path, info.Size()))
			return nil
		}
		if info.Mode().Type() != 0 {
			r.add(SeverityError, CodeUnsupportedEntryType, path,
				fmt.Sprintf("套件項目不是一般檔案或目錄（mode %s）；Runtime 無法將它具現化", info.Mode()))
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
			r.add(SeverityInfo, CodeScriptFile, path, "套件包含一個 Script；匯入或掃描期間不會執行它")
		case binaryExts[ext]:
			r.add(SeverityWarning, CodeBinaryFile, path, "套件包含一個已編譯的二進位檔；內容無法以文字方式檢視")
		case dependencyFiles[base]:
			r.add(SeverityInfo, CodeDependencyFile, path, "套件宣告了外部依賴套件")
		}

		data, err := readCapped(fsys, path, maxScanBytes)
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		if info.Size() > maxScanBytes {
			r.add(SeverityInfo, CodeFileNotScanned, path, fmt.Sprintf(
				"檔案超過內容掃描上限 %d bytes；只掃描了前 %d bytes",
				maxScanBytes, len(data)))
		}
		if isBinary(data) {

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

const maxLinkTarget = 512

func symlinkMessage(fsys fs.FS, path string, size int64) string {
	target, err := fs.ReadLink(fsys, path)
	if err != nil && size > 0 && size <= maxLinkTarget {

		if data, rerr := fs.ReadFile(fsys, path); rerr == nil && utf8.Valid(data) {
			target, err = string(data), nil
		}
	}
	what := "一個符號連結"
	if target = strings.TrimSpace(target); err == nil && target != "" {
		if len(target) > maxLinkTarget {
			target = target[:maxLinkTarget] + "…"
		}
		what = fmt.Sprintf("一個指向 %q 的符號連結", target)
	}
	return "套件項目是" + what + "，不是檔案；平台不會跟隨它，建置套件時也不會把它寫進去，" +
		"但任何把這些位元組解壓縮到磁碟的工具都會建立這個連結"
}

const urlExamplesPerHost = 3

func (r *Report) addURLDisclosures(byHost map[string][]string) {
	hosts := make([]string, 0, len(byHost))
	for h := range byHost {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		refs := byHost[h]
		sort.Strings(refs)

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
		msg := fmt.Sprintf("引用了 %d 個 %s 上的外部 URL：%s", len(refs), h, strings.Join(examples, ", "))
		if shown > len(examples) {
			msg += fmt.Sprintf("，以及另外 %d 個不同的 URL", shown-len(examples))
		}
		r.Findings = append(r.Findings, Finding{
			Severity: SeverityInfo, Code: CodeExternalURL, Message: msg, Details: refs,
		})
	}
}

func urlHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return raw
}

func (r *Report) scanSecrets(path string, data []byte) {
	for _, pat := range secretPatterns {
		if pat.Match(data) {
			r.add(SeverityError, CodePossibleSecret, path,
				"內容符合已知的憑證格式；請在匯入前移除它")
			return
		}
	}
}

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
