package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	pythonRatchetFile = "tools/python-ratchet.txt"
	pythonRatchetRoot = "tools"
)

var pythonRatchetSkippedDirs = map[string]bool{
	"node_modules": true, ".venv": true, "__pycache__": true, "target": true, "dist": true,
}

func pythonRatchetProblems(root string) []string {
	held, err := pythonRatchetBaseline(filepath.Join(root, filepath.FromSlash(pythonRatchetFile)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", pythonRatchetFile, err)}
	}
	present, err := pythonFilesUnder(root, pythonRatchetRoot)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", pythonRatchetRoot, err)}
	}
	return pythonRatchetDiff(present, held)
}

func pythonRatchetBaseline(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			held[line] = true
		}
	}
	return held, nil
}

func pythonFilesUnder(root, dir string) (map[string]bool, error) {
	found := map[string]bool{}
	base := filepath.Join(root, filepath.FromSlash(dir))
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if pythonRatchetSkippedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), extPy) {
			rel, _ := filepath.Rel(root, path)
			found[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return found, err
}

func pythonRatchetDiff(present, held map[string]bool) []string {
	var problems []string
	for _, path := range sortedKeys(present) {
		if !held[path] {
			problems = append(problems, fmt.Sprintf(
				"%s is a new Python file under %s/; tooling is written in Go (tools/devctl) or Rust, and %s only shrinks",
				path, pythonRatchetRoot, pythonRatchetFile))
		}
	}
	for _, path := range sortedKeys(held) {
		if !present[path] {
			problems = append(problems, fmt.Sprintf(
				"%s: %s no longer exists; remove its line so it cannot come back", pythonRatchetFile, path))
		}
	}
	sort.Strings(problems)
	return problems
}
