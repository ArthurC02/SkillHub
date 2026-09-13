package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	runStateMachineFile   = "apps/platform/internal/trial/execution/statemachine.go"
	runStatusConstantFile = "apps/platform/internal/foundation/persistence/db/gen/models.go"
	runTransitionTrigger  = "db/migrations/0032_run_state_and_trace_stream_guards.sql"
)

var runStatusSQLFiles = []string{"db/migrations", "db/queries"}

var (
	runTransitionRow = regexp.MustCompile(`WHEN\s+'([a-z_]+)'\s+THEN\s+NEW\.status\s+IN\s*\(([^)]*)\)`)
	runStatusInList  = regexp.MustCompile(`IN\s*\(([^)]*)\)`)
)

func runStatusSQLProblems(root string) []string {
	successors, err := goIdentListMap(
		filepath.Join(root, filepath.FromSlash(runStateMachineFile)), "successors",
		filepath.Join(root, filepath.FromSlash(runStatusConstantFile)), "RunStatus")
	if err != nil {
		return []string{fmt.Sprintf("run-status-sql: %v", err)}
	}
	all, err := goListedIdentifiers(filepath.Join(root, filepath.FromSlash(runStateMachineFile)), "AllStatuses")
	if err != nil {
		return []string{fmt.Sprintf("run-status-sql: %v", err)}
	}
	byName, err := goConstStrings(filepath.Join(root, filepath.FromSlash(runStatusConstantFile)), "RunStatus")
	if err != nil {
		return []string{fmt.Sprintf("run-status-sql: %v", err)}
	}
	known := map[string]bool{}
	for _, name := range all {
		known[byName[name]] = true
	}
	terminal := map[string]bool{}
	for value := range known {
		if _, ongoing := successors[value]; !ongoing {
			terminal[value] = true
		}
	}
	if len(successors) == 0 || len(terminal) == 0 {
		return []string{fmt.Sprintf("run-status-sql: %s yielded %d ongoing and %d terminal statuses; this comparison has lost its subject",
			runStateMachineFile, len(successors), len(terminal))}
	}

	problems := runTransitionTriggerProblems(root, successors)
	return append(problems, runTerminalListProblems(root, known, terminal)...)
}

func runTransitionTriggerProblems(root string, successors map[string]map[string]bool) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runTransitionTrigger)))
	if err != nil {
		return []string{fmt.Sprintf("run-status-sql: %v", err)}
	}
	rows := runTransitionRow.FindAllStringSubmatch(string(raw), -1)
	if len(rows) == 0 {
		return []string{fmt.Sprintf("run-status-sql: %s declares no status transition rows; the trigger moved and the Go table is now unwatched", runTransitionTrigger)}
	}
	var problems []string
	seen := map[string]bool{}
	for _, row := range rows {
		from := row[1]
		seen[from] = true
		declared, ongoing := successors[from]
		if !ongoing {
			problems = append(problems, fmt.Sprintf(
				"run-status-sql: %s lets %q move on, but the Go table treats it as terminal", runTransitionTrigger, from))
			continue
		}
		problems = append(problems, setDifference(
			fmt.Sprintf("run-status-sql: the successors of %q", from),
			fmt.Sprintf("%s's trigger", runTransitionTrigger), quotedSQLValues(row[2]),
			fmt.Sprintf("%s's successors", runStateMachineFile), declared)...)
	}
	for from := range successors {
		if !seen[from] {
			problems = append(problems, fmt.Sprintf(
				"run-status-sql: the Go table lets %q move on, but %s's trigger has no row for it, so the database would refuse the move", from, runTransitionTrigger))
		}
	}
	sort.Strings(problems)
	return problems
}

func runTerminalListProblems(root string, known, terminal map[string]bool) []string {
	var problems []string
	for _, dir := range runStatusSQLFiles {
		paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.sql"))
		if err != nil {
			return []string{fmt.Sprintf("run-status-sql: %v", err)}
		}
		sort.Strings(paths)
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				return []string{fmt.Sprintf("run-status-sql: %v", err)}
			}
			text := string(raw)
			transitionRows := map[string]bool{}
			for _, row := range runTransitionRow.FindAllString(text, -1) {
				transitionRows[row] = true
			}
			for _, match := range runStatusInList.FindAllStringSubmatchIndex(text, -1) {
				whole := text[match[0]:match[1]]
				if partOfTransitionRow(transitionRows, whole) {
					continue
				}
				values := quotedSQLValues(text[match[2]:match[3]])
				if !allRunStatuses(values, known) || !values["timed_out"] {
					continue
				}
				where := fmt.Sprintf("%s:%d", filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))), lineOf(text, match[0]))
				problems = append(problems, setDifference(
					"run-status-sql: the terminal statuses",
					where, values,
					fmt.Sprintf("%s's table", runStateMachineFile), terminal)...)
			}
		}
	}
	return problems
}

func partOfTransitionRow(rows map[string]bool, list string) bool {
	for row := range rows {
		if strings.Contains(row, list) {
			return true
		}
	}
	return false
}

func allRunStatuses(values, known map[string]bool) bool {
	for value := range values {
		if !known[value] {
			return false
		}
	}
	return len(values) > 0
}

func lineOf(text string, offset int) int {
	return strings.Count(text[:offset], "\n") + 1
}

func setDifference(subject, leftLabel string, left map[string]bool, rightLabel string, right map[string]bool) []string {
	var problems []string
	for _, value := range sortedKeys(left) {
		if !right[value] {
			problems = append(problems, fmt.Sprintf("%s: %s lists %q and %s does not", subject, leftLabel, value, rightLabel))
		}
	}
	for _, value := range sortedKeys(right) {
		if !left[value] {
			problems = append(problems, fmt.Sprintf("%s: %s lists %q and %s does not", subject, rightLabel, value, leftLabel))
		}
	}
	return problems
}

func goIdentListMap(path, varName, constPath, constType string) (map[string]map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	byName, err := goConstStrings(constPath, constType)
	if err != nil {
		return nil, err
	}
	resolve := func(expression ast.Expr) (string, bool) {
		switch identifier := expression.(type) {
		case *ast.Ident:
			value, ok := byName[identifier.Name]
			return value, ok
		case *ast.SelectorExpr:
			value, ok := byName[identifier.Sel.Name]
			return value, ok
		}
		return "", false
	}
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.VAR {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != varName || len(value.Values) != 1 {
				continue
			}
			composite, ok := value.Values[0].(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("%s: %s is not a map this check can read", path, varName)
			}
			table := map[string]map[string]bool{}
			for _, element := range composite.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					return nil, fmt.Errorf("%s: %s holds an entry this check cannot read", path, varName)
				}
				from, ok := resolve(pair.Key)
				if !ok {
					return nil, fmt.Errorf("%s: %s has a key %s does not declare", path, varName, constPath)
				}
				list, ok := pair.Value.(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("%s: %s[%s] is not a list this check can read", path, varName, from)
				}
				table[from] = map[string]bool{}
				for _, entry := range list.Elts {
					to, ok := resolve(entry)
					if !ok {
						return nil, fmt.Errorf("%s: %s[%s] holds a value %s does not declare", path, varName, from, constPath)
					}
					table[from][to] = true
				}
			}
			return table, nil
		}
	}
	return nil, fmt.Errorf("%s declares no %s; either the table moved or this check is now looking at the wrong file", path, varName)
}
