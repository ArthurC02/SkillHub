package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	commentBlockMaxLines = 3
	commentLintHint      = "go -C tools/devctl run . comment-lint"

	generatedHeaderScanBytes = 512
)

var (
	commentSourceRoots = []string{rootApps, rootPackages, rootTools, "infra", rootContracts, "db", ".github/workflows"}
	commentSkippedDirs = map[string]bool{
		dirNodeModules: true, dirDist: true, dirBuildOutput: true, "generated": true, dirGen: true, "testdata": true,
	}
	slashComments        = []string{"//"}
	cStyleComments       = []string{"//", "/*", "*"}
	hashComments         = []string{"#"}
	commentPrefixesByExt = map[string][]string{
		extGo: slashComments, extTS: cStyleComments, extTSX: cStyleComments, ".js": cStyleComments, ".mjs": cStyleComments,
		extPy: hashComments, extYML: hashComments, extYAML: hashComments, ".toml": hashComments, ".sh": hashComments,
		extSQL: {"--"},
	}
	commentMachineMarker = regexp.MustCompile(`^(//|#|--)\s*(go:|line |export |lint:|nolint|one-number:|budget-over:|` +
		`budget-ceiling:|Deprecated:|\+build|Output:|Unordered output:|eslint-|@ts-|/ <reference|@vitest-environment|` +
		`istanbul|c8 |biome-ignore|prettier-ignore|noqa|type:|pragma|fmt:|-\*-|pyright:|mypy:|shellcheck|syntax=|escape=|` +
		`check=|yaml-language-server|name:\s|!)`)
	commentWorkLog       = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,7}-\d{3}\b|\bR-\d+\b|[甲乙丙丁]-\d+|\b20\d\d-\d\d-\d\d\b|§`)
	commentWorkLogExempt = regexp.MustCompile(`\b(SHA|AES|HS|RS|ES|PS|UTF|ISO|RFC)-\d+`)
	generatedFileHeader  = regexp.MustCompile(`(?i)code generated|do not edit|auto[- ]?generated`)
)

type commentViolation struct {
	line int
	kind string
}

func commentBudgetProblems(root string) []string {
	violations, err := commentViolationsByFile(root)
	if err != nil {
		return []string{fmt.Sprintf("comment-budget: %v", err)}
	}
	var problems []string
	for path, found := range violations {
		problems = append(problems, fmt.Sprintf(
			"comment-budget: %s has %d comments to remove; see `%s %s` and AGENTS.md 慣例",
			path, len(found), commentLintHint, path))
	}
	sort.Strings(problems)
	return problems
}

func commentLint(root string, pathPrefixes []string, out io.Writer) error {
	violations, err := commentViolationsByFile(root)
	if err != nil {
		return err
	}
	var paths []string
	for path := range violations {
		if len(pathPrefixes) == 0 || hasAnyPrefix(path, pathPrefixes) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		for _, v := range violations[path] {
			fmt.Fprintf(out, "%s:%d: %s\n", path, v.line, v.kind)
		}
	}
	if len(paths) > 0 {
		return fmt.Errorf("%d files have comments to remove", len(paths))
	}
	return nil
}

type commentScan struct {
	root       string
	violations map[string][]commentViolation
}

func commentViolationsByFile(root string) (map[string][]commentViolation, error) {
	scan := commentScan{root: root, violations: map[string][]commentViolation{}}
	if err := scan.rootFiles(); err != nil {
		return nil, err
	}
	for _, top := range commentSourceRoots {
		if err := scan.tree(top); err != nil {
			return nil, err
		}
	}
	return scan.violations, nil
}

func (s commentScan) rootFiles() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := s.inspect(filepath.Join(s.root, entry.Name()), entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (s commentScan) tree(top string) error {
	err := filepath.WalkDir(filepath.Join(s.root, filepath.FromSlash(top)), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if commentSkippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		return s.inspect(path, d.Name())
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s commentScan) inspect(path, name string) error {
	prefixes, ok := commentPrefixesFor(name)
	if !ok {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if generatedFileHeader.Match(src[:min(len(src), generatedHeaderScanBytes)]) {
		return nil
	}
	if found := commentViolations(string(src), prefixes); len(found) > 0 {
		s.violations[harnessRelative(s.root, path)] = found
	}
	return nil
}

func commentPrefixesFor(name string) ([]string, bool) {
	switch {
	case strings.HasSuffix(name, ".d.ts"):
		return nil, false
	case name == ".env.example", strings.HasPrefix(name, "Dockerfile"), strings.HasSuffix(name, ".Dockerfile"):
		return hashComments, true
	}
	prefixes, ok := commentPrefixesByExt[filepath.Ext(name)]
	return prefixes, ok
}

// commentViolations scans line-leading comments only; markers and bare /* */ delimiters
// continue a block without counting toward its length.
func commentViolations(src string, prefixes []string) []commentViolation {
	var found []commentViolation
	start, counted := 0, 0
	closeBlock := func() {
		if counted > commentBlockMaxLines {
			found = append(found, commentViolation{start, fmt.Sprintf("comment block over %d lines", commentBlockMaxLines)})
		}
		start, counted = 0, 0
	}
	for index, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !hasAnyPrefix(trimmed, prefixes) {
			closeBlock()
			continue
		}
		if start == 0 {
			start = index + 1
		}
		if commentMachineMarker.MatchString(trimmed) || trimmed == "/*" || trimmed == "/**" || trimmed == "*/" {
			continue
		}
		counted++
		if commentWorkLog.MatchString(commentWorkLogExempt.ReplaceAllString(trimmed, "")) {
			found = append(found, commentViolation{index + 1, "work log in a comment (requirement or ruling id, date, §)"})
		}
	}
	closeBlock()
	sort.Slice(found, func(i, j int) bool { return found[i].line < found[j].line })
	return found
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
