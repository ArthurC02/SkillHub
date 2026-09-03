package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// A markdown link whose target does not exist fails CI.
//
// # Why this exists
//
// The 2026-09-03 documentation patrol resolved every relative link in the 219
// tracked markdown files and found sixteen that pointed at nothing. Fifteen of
// them were ADR references, and the failure mode was always the same: somebody
// wrote a filename that describes the ADR correctly — ADR-011 as
// `workspace-scope-and-tenancy`, ADR-018 as `data-platform-and-storage` — while
// the file on disk is called something else. One of them had the number wrong
// too, pointing at Local Runner for a sentence about the Provider Port.
//
// None of that is visible while reading. The link text is right, the number is
// right, the filename reads like the subject, and the only way to learn it is
// dead is to click it — which nobody does inside a repository. A checker resolves
// all of them in a few milliseconds, and it is the only kind of documentation
// defect in this repo that is fully mechanical: no judgement, no false positives
// that a human has to arbitrate.
//
// # What it does not check
//
// Only whether the path exists. Not the anchor after `#`, not whether the target
// still says what the link claims, not http(s) reachability — that last one would
// make CI depend on the internet, and a 404 on somebody else's server is not this
// repository's defect.
var docLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// Directories this walk does not enter, each with the reason it is out of scope.
// This is a stock list, not an extension point: a new entry means a new class of
// markdown nobody is allowed to fix, so it needs the same argument these four had.
var docLinkSkippedDirs = map[string]string{
	// Not hand-written: the OpenAPI generators emit their own README trees, and
	// generated files are never hand-edited (AGENTS.md 開發自動化 5).
	"packages": "generated API clients",
	".devctl":  "codegen scratch output",
	// Dependencies and build output.
	"node_modules": "dependencies",
	"__pycache__":  "build output",
	".git":         "git internals",
}

// tools/goldenset/corpus is the one deliberate exception, and it is a path rather
// than a directory name because only that subtree earns it: the corpus is 31
// pinned upstream SKILL.md files with no bundled `references/` directory ever
// captured, so their internal links dangle by design. They are also frozen gate
// evidence (tools/goldenset/README.md) — changing them would move the recall
// number the M1 gate was judged on, so "fixing" these links is forbidden, not
// merely unnecessary.
const docLinkFrozenCorpus = "tools/goldenset/corpus"

func docLinkProblems(root string) []string {
	var problems []string
	_ = filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relative := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(p, root), string(filepath.Separator)))
		if entry.IsDir() {
			if _, skipped := docLinkSkippedDirs[entry.Name()]; skipped {
				return fs.SkipDir
			}
			if relative == docLinkFrozenCorpus {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		body, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		dir := filepath.Dir(p)
		for i, line := range strings.Split(string(body), "\n") {
			for _, match := range docLinkPattern.FindAllStringSubmatch(line, -1) {
				target, ok := docLinkTarget(match[1])
				if !ok {
					continue
				}
				if _, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(target))); statErr != nil {
					problems = append(problems, fmt.Sprintf(
						"doc-links: %s:%d links to %q and no such file exists. Check the real filename "+
							"(`ls` the directory) rather than the one the subject suggests — every dead "+
							"link found on 2026-09-03 read correctly and pointed at nothing",
						relative, i+1, match[1]))
				}
			}
		}
		return nil
	})
	return problems
}

// docLinkTarget returns the filesystem path a markdown link resolves to, and
// whether this checker has anything to say about it. Absolute URLs, mail links
// and bare anchors are somebody else's problem or nobody's.
func docLinkTarget(raw string) (string, bool) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") ||
		strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "<") {
		return "", false
	}
	target := raw
	if hash := strings.Index(target, "#"); hash >= 0 {
		target = target[:hash]
	}
	if decoded, err := url.PathUnescape(target); err == nil {
		target = decoded
	}
	if target == "" || path.IsAbs(target) {
		return "", false
	}
	// A target that is nothing but dots is a placeholder in prose, not a path.
	// The one in this repo is AGENTS.md's rule for writing an ADR reference,
	// `→ [ADR-xxx](...)`, where the parentheses are the blank to fill in.
	//
	// It is also the reason this checker's first CI run failed while every local
	// run passed: Windows resolves `...` and Linux does not, so a false positive
	// that is invisible on the development machine is a hard failure in CI. Any
	// checker that asks the filesystem a question has this asymmetry; this one
	// now answers it before asking.
	if strings.Trim(target, ".") == "" {
		return "", false
	}
	return target, true
}
