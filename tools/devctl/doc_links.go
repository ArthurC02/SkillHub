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

var docLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

var docLinkSkippedDirs = map[string]string{

	rootPackages:   "generated API clients",
	dirDevctlCache: "codegen scratch output",

	dirNodeModules: "dependencies",
	dirPycache:     "build output",
	dirGit:         "git internals",
	"worktrees":    "other checkouts of this repository",
}

const docLinkFrozenCorpus = "tools/goldenset/corpus"

// A file named "<id>.SKILL.md" or "<id>-<arm>.SKILL.md" is a model-generated
// Skill body kept as evidence; its links point into the model's own output,
// not this repository.
const docLinkModelDumpSuffix = ".SKILL.md"

func docLinkProblems(root string) []string {
	var problems []string
	_ = filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return continuePastUnreadableEntry()
		}
		relative := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(p, root), string(filepath.Separator)))
		if entry.IsDir() {
			if _, skipped := docLinkSkippedDirs[entry.Name()]; skipped || relative == docLinkFrozenCorpus {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") || strings.HasSuffix(entry.Name(), docLinkModelDumpSuffix) {
			return nil
		}
		body, readErr := os.ReadFile(p)
		if readErr != nil {
			return continuePastUnreadableEntry()
		}
		problems = append(problems, deadDocLinks(filepath.Dir(p), relative, string(body))...)
		return nil
	})
	return problems
}

func deadDocLinks(dir, relative, body string) []string {
	var problems []string
	for i, line := range strings.Split(body, "\n") {
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
	return problems
}

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

	// A target that is only dots is a prose placeholder, not a path: Windows
	// resolves "..." to a real directory while Linux does not, so this must
	// be caught before the filesystem is asked.
	if strings.Trim(target, ".") == "" {
		return "", false
	}
	return target, true
}
