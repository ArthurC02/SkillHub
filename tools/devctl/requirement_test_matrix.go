package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	requirementMatrixPath = "docs/plans/requirement-test-matrix.md"
	requirementSpecPath   = "docs/plans/02-specifications-and-acceptance-criteria.md"
)

var matrixStatuses = map[string]bool{
	"有測試": true, "部分": true, "待真機": true, "待真人": true, "未實作": true,
}

var (
	matrixHeading  = regexp.MustCompile(`^#{2,6}\s+(.*)$`)
	matrixRow      = regexp.MustCompile("^\\|\\s*([A-Z][A-Z0-9]{1,7}-\\d{3})\\s*\\|([^|]*)\\|([^|]*)\\|([^|]*)\\|\\s*$")
	matrixQuoted   = regexp.MustCompile("`([^`]+)`")
	matrixTestFile = regexp.MustCompile(`(_test\.go|\.test\.tsx?|_test\.py|_test\.sql)$`)
)

func specRequirementIDs(root string) (required map[string]bool, postMVP map[string]bool, err error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(requirementSpecPath)))
	if err != nil {
		return nil, nil, err
	}
	required, postMVP = map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		heading := requirementHeading.FindStringSubmatch(line)
		if heading == nil {
			continue
		}
		ids := requirementID.FindAllString(heading[2], -1)
		if len(ids) == 0 {
			continue
		}
		if strings.Contains(line, "後 MVP") {
			postMVP[ids[0]] = true
			continue
		}
		required[ids[0]] = true
	}
	return required, postMVP, nil
}

func testCorpus(root string) (map[string]bool, string) {
	paths := map[string]bool{}
	var corpus strings.Builder
	skip := map[string]bool{".git": true, "node_modules": true, ".venv": true, ".devctl": true, "dist": true, "__pycache__": true}

	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		paths[filepath.ToSlash(relative)] = true
		name := entry.Name()
		if !matrixTestFile.MatchString(name) && !strings.HasPrefix(name, "test_") {
			return nil
		}
		if body, err := os.ReadFile(path); err == nil {
			corpus.Write(body)
			corpus.WriteString("\n")
		}
		return nil
	})
	return paths, corpus.String()
}

func requirementTestMatrixProblems(root string) []string {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(requirementMatrixPath)))
	if err != nil {
		return []string{fmt.Sprintf("requirement-test-matrix: cannot read %s: %v", requirementMatrixPath, err)}
	}
	required, postMVP, err := specRequirementIDs(root)
	if err != nil {
		return []string{fmt.Sprintf("requirement-test-matrix: cannot read %s: %v", requirementSpecPath, err)}
	}
	if len(required) == 0 {
		return []string{fmt.Sprintf("requirement-test-matrix: %s names no MVP requirement; this check has lost its subject", requirementSpecPath)}
	}
	paths, corpus := testCorpus(root)

	var problems []string
	listed := map[string]int{}
	for number, line := range strings.Split(string(body), "\n") {
		row := matrixRow.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		id, status := row[1], strings.TrimSpace(row[2])
		tests, gap := strings.TrimSpace(row[3]), strings.TrimSpace(row[4])
		at := fmt.Sprintf("%s:%d", requirementMatrixPath, number+1)
		listed[id]++

		switch {
		case postMVP[id]:
			problems = append(problems, fmt.Sprintf(
				"requirement-test-matrix: %s lists %s, which %s marks 後 MVP; this table is the MVP-required set", at, id, requirementSpecPath))
		case !required[id]:
			problems = append(problems, fmt.Sprintf(
				"requirement-test-matrix: %s lists %s, which is not a requirement heading in %s", at, id, requirementSpecPath))
		}
		if !matrixStatuses[status] {
			problems = append(problems, fmt.Sprintf(
				"requirement-test-matrix: %s gives %s the status %q, which is not one of the five the table declares", at, id, status))
		}

		named := matrixQuoted.FindAllStringSubmatch(tests, -1)
		if status == "有測試" {
			if len(named) == 0 {
				problems = append(problems, fmt.Sprintf(
					"requirement-test-matrix: %s calls %s 有測試 but names no test", at, id))
			}
			if gap != "" {
				problems = append(problems, fmt.Sprintf(
					"requirement-test-matrix: %s calls %s 有測試 yet states a gap; a requirement with a gap is 部分 or one of the three 待/未 states", at, id))
			}
		} else if gap == "" {
			problems = append(problems, fmt.Sprintf(
				"requirement-test-matrix: %s gives %s the status %q without saying what is missing", at, id, status))
		}

		for _, quoted := range named {
			name := quoted[1]
			if paths[name] || strings.Contains(corpus, name) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"requirement-test-matrix: %s names %q for %s, but no test file contains it and no such file exists", at, name, id))
		}
	}

	missing := make([]string, 0, len(required))
	for id := range required {
		if listed[id] == 0 {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		problems = append(problems, fmt.Sprintf(
			"requirement-test-matrix: %s is MVP-required in %s but has no row in %s", id, requirementSpecPath, requirementMatrixPath))
	}

	duplicated := make([]string, 0, len(listed))
	for id, count := range listed {
		if count > 1 {
			duplicated = append(duplicated, fmt.Sprintf("%s (%d rows)", id, count))
		}
	}
	sort.Strings(duplicated)
	for _, id := range duplicated {
		problems = append(problems, fmt.Sprintf("requirement-test-matrix: %s appears more than once: %s", requirementMatrixPath, id))
	}
	return problems
}
