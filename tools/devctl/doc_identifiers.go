package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var docIdentifierScope = []string{
	"AGENTS.md",
	"docs/plans/01-goals-and-plan.md",
	"docs/plans/02-specifications-and-acceptance-criteria.md",
	"docs/plans/03-work-items.md",
	"docs/plans/04-backlog-and-handoffs.md",
	"docs/plans/05-pending-rulings.md",
	"docs/design/system.md",
	"docs/design/information-architecture.md",
	"docs/plans/mvp/m5/README.md",
}

var docIdentifierPattern = regexp.MustCompile("`(Test[A-Za-z0-9_]{3,}|test_[a-z0-9_]{3,}|[A-Z][A-Za-z0-9]{4,})`")

var allowedDocWords = map[string]string{
	"Superseded":               "ADR status vocabulary (AGENTS.md), not a symbol",
	"Proposed":                 "ADR status vocabulary",
	"Accepted":                 "ADR status vocabulary",
	"FileCountLimit":           "tail of an elided list: TestDatasetUploadEnforcesPerFileSizeLimit／FileCountLimit／TotalSizeLimit",
	"TotalSizeLimit":           "tail of the same elided list",
	"Deallocate":               "pgx / Postgres protocol message, not a SkillHub symbol",
	"MaxConnLifetime":          "pgxpool.Config field, not a SkillHub symbol",
	"QueryExecModeExec":        "pgx query exec mode, not a SkillHub symbol",
	"ReadyForQuery":            "Postgres wire-protocol message",
	"NOTIFY":                   "Postgres command",
	"ModuleNotFoundError":      "Python builtin exception",
	"test_cases_skill_id_fkey": "constraint name Postgres generates for the test_cases foreign key",
	"Querier":                  "sqlc interface that db/sqlc.yaml deliberately does not emit",
	"MARKER":                   "shell variable in tools/sec009 (.sh is outside codeExtensions)",
}

var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".py": true, ".sql": true, ".yaml": true, ".yml": true, ".json": true,
}

func docIdentifierProblems(root string) []string {
	declared := map[string]bool{}
	word := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)
	skip := map[string]bool{".git": true, "node_modules": true, ".venv": true, ".devctl": true, "dist": true, "__pycache__": true}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		if filepath.Base(path) == "doc_identifiers.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, w := range word.FindAllString(string(body), -1) {
			declared[w] = true
		}
		return nil
	})

	missing := map[string][]string{}
	for _, rel := range docIdentifierScope {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, m := range docIdentifierPattern.FindAllStringSubmatch(string(body), -1) {
			name := m[1]
			if declared[name] || allowedDocWords[name] != "" {
				continue
			}
			if !contains(missing[name], rel) {
				missing[name] = append(missing[name], rel)
			}
		}
	}

	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)

	var problems []string
	for _, n := range names {
		problems = append(problems, fmt.Sprintf(
			"doc-identifier: %s is named in %s but declared in no file. "+
				"Correct the document, or add it to allowedDocWords with the reason it is prose.",
			n, strings.Join(missing[n], ", ")))
	}
	return problems
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
