package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const characterizationScriptGlob = "tools/*/test_*_characterization.py"

func characterizationMainProblems(root string) []string {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(characterizationScriptGlob)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", characterizationScriptGlob, err)}
	}
	sort.Strings(paths)
	var problems []string
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		rel, _ := filepath.Rel(root, path)
		problems = append(problems, testsDefinedAfterMain(filepath.ToSlash(rel), string(src))...)
	}
	return problems
}

func testsDefinedAfterMain(file, src string) []string {
	mainAt := -1
	var problems []string
	for i, line := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(line, "if __name__ == \"__main__\":"), strings.HasPrefix(line, "if __name__ == '__main__':"):
			mainAt = i + 1
		case mainAt > 0 && strings.HasPrefix(line, "def test_"):
			name := strings.TrimPrefix(line[:strings.IndexByte(line, '(')], "def ")
			problems = append(problems, fmt.Sprintf(
				"%s:%d: %s is defined after the __main__ block on line %d, so running the script never collects it; move it above the block",
				file, i+1, name, mainAt))
		}
	}
	return problems
}
