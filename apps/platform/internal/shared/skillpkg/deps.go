package skillpkg

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Dependency extraction: what a package's own code says it needs.
//
// The M2 baseline found the catalog's `deps` field short by 8 packages across
// 13 of 45 skills, and the Runtime Image — which installs the union of that
// field — inherited every gap (content-baseline-report.md §13.4). Nothing
// caught it, because `deps` was a human transcription of each SKILL.md and the
// only test of it was a Run failing at the ModuleNotFoundError, in a sandbox
// where `pip` is deliberately absent and no fix is possible.
//
// So the scan states the answer mechanically. Two findings, because there are
// two different questions:
//
//   package-dependencies (info) — the third-party packages this package
//     evidences, from imports and from its own install lines. This is the value
//     CONTENT-003 fills `deps` from, and the reason that criterion can say
//     "verifiable by static scan" instead of "please read carefully".
//
//   undeclared-dependency (warning) — code imports something the package never
//     declares anywhere: no requirements.txt, no package.json, no install line.
//     A reader of such a package cannot find out what it needs without reading
//     every script, and the curator transcribing `deps` had nothing to copy.
//
// Both are static: nothing here executes, imports, or resolves anything (iron
// rule 1, CONTENT-006 "匯入與掃描階段不執行套件內任何 Script").
//
// ponytail: regex extraction, not a Python/JS parse. It reads import lines,
// which is what the packages write; a package hiding a dependency behind
// importlib.import_module(name) is not something a scanner should pretend to
// see, and the finding is advisory rather than blocking for exactly that
// reason. A real parser only if obfuscated imports ever become a real case.

var (
	// `import a.b, c` and `from a.b import c`, anchored to the start of a line
	// so prose does not count. Submodules are cut to the top-level name, which
	// is what actually gets installed.
	//
	// The `from` form insists on the `import` keyword after the module. Without
	// it the pattern matches ordinary English — "from the model API" scored
	// `the` as a dependency of two seed packages.
	pyImportRe     = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([A-Za-z_][A-Za-z0-9_.]*(?:[ \t]*,[ \t]*[A-Za-z_][A-Za-z0-9_.]*)*)`)
	pyFromImportRe = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]+import[ \t]`)
	// require("x") and ES `from "x"`, the two forms the seed's Node skills use.
	jsImportRe = regexp.MustCompile(`(?:require\([ \t]*|from[ \t]+)["']([^"']+)["']`)
	// Install lines in documentation. These are a declaration, not an
	// instruction the platform will follow: the sandbox has no network and no
	// package manager, so an install line is only ever evidence of a need.
	//
	// Anchored to the start of a line (a shell prompt aside), because the same
	// verb appears mid-sentence: "do not run `npm install` first ... The model
	// knows the API" scored `the`, `model`, `knows` and `api` as dependencies.
	// A real install line stands alone; one quoted inside a paragraph is prose.
	installRe = regexp.MustCompile(`(?m)^[ \t]*(?:[$>][ \t]+)?(?:pip3?|uv pip|npm|pnpm|yarn)[ \t]+(?:install|add)[ \t]+([^\n|;&` + "`" + `]+)`)
	// A PyPI/npm distribution name, used to reject shell flags and prose that
	// follow an install verb.
	distNameRe = regexp.MustCompile(`^(?:@[A-Za-z0-9._-]+/)?[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// depScan accumulates dependency evidence across a package's files.
type depScan struct {
	imported map[string]bool // third-party modules the code imports
	declared map[string]bool // packages the package names in a manifest or install line
	local    map[string]bool // module names the package ships itself
	manifest bool            // a dependency manifest is present
}

func newDepScan() *depScan {
	return &depScan{
		imported: map[string]bool{},
		declared: map[string]bool{},
		local:    map[string]bool{},
	}
}

// note records the module names a package file itself provides, so a script
// importing its sibling is not reported as needing a third-party package. Called
// for every file, including ones too large to scan: what a file is named is
// known without reading it.
func (d *depScan) note(p string) {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg != "" && seg != "." {
			d.local[seg] = true
		}
	}
	base := path.Base(p)
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	d.local[base] = true
}

// observe extracts dependency evidence from one file's content.
func (d *depScan) observe(p, content string) {
	switch strings.ToLower(path.Ext(p)) {
	case ".py":
		d.observePython(content)
	case ".js", ".mjs", ".cjs", ".ts":
		d.observeJS(content)
	case ".md", ".markdown":
		// SKILL.md is where the prompt-only skills declare everything they
		// need: three of them named pycountry and chardet on an install line
		// while the catalog recorded only pandas.
		d.observeInstallLines(content)
		forEachFence(content, func(lang, _, body string) {
			switch lang {
			case "python":
				d.observePython(body)
			case "javascript", "typescript":
				d.observeJS(body)
			case "bash":
				d.observeInstallLines(body)
			}
		})
	}

	switch strings.ToLower(path.Base(p)) {
	case "requirements.txt":
		d.manifest = true
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(cutComment(line))
			if name := strings.TrimSpace(splitAny(line, "=<>~!;[ ")); distNameRe.MatchString(name) {
				d.declared[normalizeDist(name)] = true
			}
		}
	case "package.json", "pyproject.toml", "gemfile", "cargo.toml", "go.mod", "pom.xml":
		// Not parsed: the name-to-module mapping inside these is format
		// specific, and their mere presence is what the check needs — a package
		// carrying a manifest has somewhere to state its dependencies, so an
		// import missing from it is the manifest's problem to fix, not an
		// undisclosed need. scanTree already discloses the file itself.
		d.manifest = true
	}
}

func (d *depScan) observePython(content string) {
	add := func(name string) {
		top, _, _ := strings.Cut(strings.TrimSpace(name), ".")
		if top == "" || pyStdlib[top] || strings.HasPrefix(top, "_") {
			return
		}
		d.imported[top] = true
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(content, -1) {
		for _, name := range strings.Split(m[1], ",") {
			add(name)
		}
	}
	for _, m := range pyFromImportRe.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
}

func (d *depScan) observeJS(content string) {
	for _, m := range jsImportRe.FindAllStringSubmatch(content, -1) {
		spec := m[1]
		if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") ||
			strings.HasPrefix(spec, "node:") || nodeBuiltin[spec] {
			continue
		}
		// Scoped packages keep two segments (@scope/name), others keep one.
		parts := strings.Split(spec, "/")
		name := parts[0]
		if strings.HasPrefix(spec, "@") && len(parts) > 1 {
			name = parts[0] + "/" + parts[1]
		}
		d.imported[name] = true
	}
}

func (d *depScan) observeInstallLines(content string) {
	for _, line := range strings.Split(content, "\n") {
		for _, m := range installRe.FindAllStringSubmatch(cutComment(line), -1) {
			for _, word := range strings.Fields(m[1]) {
				if strings.HasPrefix(word, "-") {
					continue
				}
				name := splitAny(word, "=<>~!@[")
				if name != "" && distNameRe.MatchString(name) {
					d.declared[normalizeDist(name)] = true
				}
			}
		}
	}
}

// report emits the two findings. Names are compared in their distribution form,
// so a script importing `docx` is matched by a declaration of `python-docx`.
func (d *depScan) report(r *Report) {
	thirdParty := map[string]bool{}
	for name := range d.imported {
		if !d.local[name] {
			thirdParty[normalizeDist(name)] = true
		}
	}

	all := union(thirdParty, d.declared)
	if len(all) == 0 {
		return
	}
	// The names go in Details as a list, not only into the sentence: this is the
	// value CONTENT-003 fills `deps` from, and a consumer should not have to
	// parse a message to get it (same reason external-url carries its URLs).
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityInfo, Code: CodePackageDependencies, Path: "SKILL.md",
		Message: fmt.Sprintf(
			"套件顯示出 %d 個第三方依賴套件：%s。"+
				"這是從 import 陳述式與安裝指令行讀出的，不曾執行任何東西；"+
				"Runtime Image 會另外決定實際承載哪些。",
			len(all), strings.Join(all, ", ")),
		Details: all,
	})

	if d.manifest {
		return
	}
	undeclared := difference(thirdParty, d.declared)
	if len(undeclared) == 0 {
		return
	}
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityWarning, Code: CodeUndeclaredDependency, Path: "SKILL.md",
		Message: fmt.Sprintf(
			"程式碼匯入了 %d 個套件從未宣告的依賴套件——沒有依賴清單檔，"+
				"也沒有任何安裝指令行提到 %s。安裝 Runtime 的人若不逐一讀過每支 Script，"+
				"無從得知這些依賴。",
			len(undeclared), strings.Join(undeclared, ", ")),
		Details: undeclared,
	})
}

// normalizeDist maps an import name to the distribution name that installs it,
// where the two differ. Only well-established pairs: guessing would make the
// comparison claim a declaration covers an import when it does not.
var importToDist = map[string]string{
	"docx": "python-docx", "pptx": "python-pptx", "dateutil": "python-dateutil",
	"PIL": "pillow", "fitz": "PyMuPDF", "yaml": "PyYAML", "bs4": "beautifulsoup4",
	"sklearn": "scikit-learn", "cv2": "opencv-python", "stdnum": "python-stdnum",
	"pdfminer": "pdfminer.six", "win32com": "pywin32", "pythoncom": "pywin32",
	"pywintypes": "pywin32", "charset_normalizer": "charset-normalizer",
	"confusable_homoglyphs": "confusable-homoglyphs", "dotenv": "python-dotenv",
	"magic": "python-magic", "attr": "attrs", "OpenSSL": "pyOpenSSL",
}

func normalizeDist(name string) string {
	if d, ok := importToDist[name]; ok {
		return d
	}
	// PyPI treats `_` and `-` as the same character in a distribution name.
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func cutComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

// splitAny returns the prefix of s before the first byte in cutset.
func splitAny(s, cutset string) string {
	if i := strings.IndexAny(s, cutset); i >= 0 {
		return s[:i]
	}
	return s
}

func union(a, b map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]bool{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// nodeBuiltin covers the bare (un-prefixed) spellings of Node's builtins. The
// `node:` prefixed form is handled by prefix and needs no list.
var nodeBuiltin = map[string]bool{
	"assert": true, "buffer": true, "child_process": true, "cluster": true,
	"console": true, "constants": true, "crypto": true, "dgram": true,
	"diagnostics_channel": true, "dns": true, "domain": true, "events": true,
	"fs": true, "fs/promises": true, "http": true, "http2": true, "https": true,
	"inspector": true, "module": true, "net": true, "os": true, "path": true,
	"perf_hooks": true, "process": true, "punycode": true, "querystring": true,
	"readline": true, "repl": true, "stream": true, "string_decoder": true,
	"sys": true, "timers": true, "tls": true, "trace_events": true, "tty": true,
	"url": true, "util": true, "v8": true, "vm": true, "wasi": true,
	"worker_threads": true, "zlib": true,
}

// pyStdlib is CPython 3.11's top-level module set — the interpreter version the
// Runtime Image ships. A name missing here becomes a false "undeclared
// dependency", which is why this is the published list rather than a sample of
// the common ones.
var pyStdlib = map[string]bool{
	"abc": true, "aifc": true, "argparse": true, "array": true, "ast": true,
	"asynchat": true, "asyncio": true, "asyncore": true, "atexit": true,
	"audioop": true, "base64": true, "bdb": true, "binascii": true,
	"bisect": true, "builtins": true, "bz2": true, "calendar": true,
	"cgi": true, "cgitb": true, "chunk": true, "cmath": true, "cmd": true,
	"code": true, "codecs": true, "codeop": true, "collections": true,
	"colorsys": true, "compileall": true, "concurrent": true, "configparser": true,
	"contextlib": true, "contextvars": true, "copy": true, "copyreg": true,
	"cProfile": true, "crypt": true, "csv": true, "ctypes": true, "curses": true,
	"dataclasses": true, "datetime": true, "dbm": true, "decimal": true,
	"difflib": true, "dis": true, "doctest": true, "email": true,
	"encodings": true, "ensurepip": true, "enum": true, "errno": true,
	"faulthandler": true, "fcntl": true, "filecmp": true, "fileinput": true,
	"fnmatch": true, "fractions": true, "ftplib": true, "functools": true,
	"gc": true, "getopt": true, "getpass": true, "gettext": true, "glob": true,
	"graphlib": true, "grp": true, "gzip": true, "hashlib": true, "heapq": true,
	"hmac": true, "html": true, "http": true, "idlelib": true, "imaplib": true,
	"imghdr": true, "imp": true, "importlib": true, "inspect": true, "io": true,
	"ipaddress": true, "itertools": true, "json": true, "keyword": true,
	"lib2to3": true, "linecache": true, "locale": true, "logging": true,
	"lzma": true, "mailbox": true, "mailcap": true, "marshal": true,
	"math": true, "mimetypes": true, "mmap": true, "modulefinder": true,
	"msilib": true, "msvcrt": true, "multiprocessing": true, "netrc": true,
	"nis": true, "nntplib": true, "ntpath": true, "numbers": true,
	"operator": true, "optparse": true, "os": true, "ossaudiodev": true,
	"pathlib": true, "pdb": true, "pickle": true, "pickletools": true,
	"pipes": true, "pkgutil": true, "platform": true, "plistlib": true,
	"poplib": true, "posixpath": true, "pprint": true, "profile": true,
	"pstats": true, "pty": true, "pwd": true, "py_compile": true,
	"pyclbr": true, "pydoc": true, "queue": true, "quopri": true, "random": true,
	"re": true, "readline": true, "reprlib": true, "resource": true,
	"rlcompleter": true, "runpy": true, "sched": true, "secrets": true,
	"select": true, "selectors": true, "shelve": true, "shlex": true,
	"shutil": true, "signal": true, "site": true, "smtplib": true, "sndhdr": true,
	"socket": true, "socketserver": true, "spwd": true, "sqlite3": true,
	"sre_compile": true, "sre_constants": true, "sre_parse": true, "ssl": true,
	"stat": true, "statistics": true, "string": true, "stringprep": true,
	"struct": true, "subprocess": true, "sunau": true, "symtable": true,
	"sys": true, "sysconfig": true, "syslog": true, "tabnanny": true,
	"tarfile": true, "telnetlib": true, "tempfile": true, "termios": true,
	"test": true, "textwrap": true, "threading": true, "time": true,
	"timeit": true, "tkinter": true, "token": true, "tokenize": true,
	"tomllib": true, "trace": true, "traceback": true, "tracemalloc": true,
	"tty": true, "turtle": true, "turtledemo": true, "types": true,
	"typing": true, "unicodedata": true, "unittest": true, "urllib": true,
	"uu": true, "uuid": true, "venv": true, "warnings": true, "wave": true,
	"weakref": true, "webbrowser": true, "winreg": true, "winsound": true,
	"wsgiref": true, "xdrlib": true, "xml": true, "xmlrpc": true,
	"zipapp": true, "zipfile": true, "zipimport": true, "zlib": true,
	"zoneinfo": true,
}
