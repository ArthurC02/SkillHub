package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var docProseSkipDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, ".devctl": true,
	"dist": true, "__pycache__": true, ".agents": true, ".codex": true,
}

var docProseSkipTrees = []string{
	"docs/plans/mvp",
	"tools/goldenset/corpus",
}

const (
	removalFix = "A sentence that lost an identifier keeps its grammar; " +
		"rewrite it so it says the same thing without the name that went away."
	widthFix = "Chinese prose here is written with full-width marks; a half-width one next to " +
		"a Chinese character came from the keyboard, not from the sentence. Digits keep theirs."
)

var docProseShapes = []struct {
	name    string
	pattern *regexp.Regexp
	fix     string
}{
	{"a control character", regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`), removalFix},
	{"two spaces between CJK characters", regexp.MustCompile(`[\x{3000}-\x{9fff}]  +[\x{3000}-\x{9fff}\x{300c}\x{ff08}]`), removalFix},
	{"a space before closing punctuation", regexp.MustCompile(`[\x{4e00}-\x{9fff}] +[\x{ff0c}\x{3002}\x{ff1b}\x{3001}\x{ff09}\x{300d}]`), removalFix},
	{"doubled punctuation", regexp.MustCompile(`[\x{ff0c}\x{3002}\x{ff1b}\x{3001}]{2,}`), removalFix},
	{"an opening bracket followed by punctuation", regexp.MustCompile(`\x{ff08}[\x{ff0c}\x{3002}\x{3001}\x{ff1b}\x{ff09}]`), removalFix},
	{"half-width punctuation in a Chinese sentence", regexp.MustCompile(`[\x{4e00}-\x{9fff}\x{300d}\x{300f}\x{ff09}\x{3011}\x{300b}][,;]|[^0-9][,;][\x{4e00}-\x{9fff}]`), widthFix},
}

var docProseBoxDrawing = regexp.MustCompile(`[\x{2500}-\x{257f}]`)

var docProseInlineCode = regexp.MustCompile("`[^`\n]*`")

func docProseMaskInlineCode(line string) string {
	masked := []byte(line)
	for _, at := range docProseInlineCode.FindAllStringIndex(line, -1) {
		for i := at[0]; i < at[1]; i++ {
			masked[i] = '_'
		}
	}
	return string(masked)
}

func docProseProblems(root string) []string {
	var problems []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case entry.IsDir():
			if docProseSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		case !strings.HasSuffix(entry.Name(), ".md"):
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		slashed := filepath.ToSlash(relative)
		for _, tree := range docProseSkipTrees {
			if strings.HasPrefix(slashed, tree+"/") {
				return nil
			}
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		problems = append(problems, docProseLineProblems(slashed, string(body))...)
		return nil
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("doc-prose: %v", err))
	}
	sort.Strings(problems)
	return problems
}

func docProseLineProblems(relative, body string) []string {
	var problems []string
	fenced := false
	for number, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, "> "), "```") {
			fenced = !fenced
			continue
		}
		if fenced || docProseBoxDrawing.MatchString(line) {
			continue
		}
		scanned := docProseMaskInlineCode(line)
		for _, shape := range docProseShapes {
			at := shape.pattern.FindStringIndex(scanned)
			if at == nil {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"doc-prose: %s:%d has %s: %q. %s",
				relative, number+1, shape.name, docProseExcerpt(line, at[0], at[1]), shape.fix))
		}
	}
	return problems
}

func docProseExcerpt(line string, from, to int) string {
	runes := []rune(line)
	start := len([]rune(line[:from])) - 12
	if start < 0 {
		start = 0
	}
	end := len([]rune(line[:to])) + 12
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}
