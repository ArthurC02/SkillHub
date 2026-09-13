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
	"strconv"
	"strings"
)

type vocabularySource struct {
	label string
	read  func(root string) (map[string]bool, error)
}

type domainVocabulary struct {
	name    string
	sources []vocabularySource
	absent  string
}

func goConstEnum(path, typeName string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (%s constants)", path, typeName),
		read: func(root string) (map[string]bool, error) {
			byName, err := goConstStrings(filepath.Join(root, filepath.FromSlash(path)), typeName)
			if err != nil {
				return nil, err
			}
			values := map[string]bool{}
			for _, value := range byName {
				values[value] = true
			}
			return values, nil
		},
	}
}

func goListedConstEnum(listPath, listName, constPath, constType string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (%s)", listPath, listName),
		read: func(root string) (map[string]bool, error) {
			names, err := goListedIdentifiers(filepath.Join(root, filepath.FromSlash(listPath)), listName)
			if err != nil {
				return nil, err
			}
			byName, err := goConstStrings(filepath.Join(root, filepath.FromSlash(constPath)), constType)
			if err != nil {
				return nil, err
			}
			values := map[string]bool{}
			for _, name := range names {
				value, declared := byName[name]
				if !declared {
					return nil, fmt.Errorf("%s: %s lists %s, which %s does not declare", listPath, listName, name, constPath)
				}
				values[value] = true
			}
			return values, nil
		},
	}
}

func sqlCheckIn(path, column string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (CHECK on %s)", path, column),
		read: func(root string) (map[string]bool, error) {
			return sqlCheckValues(filepath.Join(root, filepath.FromSlash(path)), column)
		},
	}
}

func postgresEnum(path, typeName string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (enum %s)", path, typeName),
		read: func(root string) (map[string]bool, error) {
			return postgresEnumValues(filepath.Join(root, filepath.FromSlash(path)), typeName)
		},
	}
}

var domainVocabularies = []domainVocabulary{
	{
		name: "run status",
		sources: []vocabularySource{
			postgresEnum("db/migrations/0004_test_lab_and_runs.sql", "run_status"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "RunStatus"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/statemachine.go", "AllStatuses",
				"apps/platform/internal/foundation/persistence/db/gen/models.go", "RunStatus"),
		},
	},
	{
		name: "creation session state",
		sources: []vocabularySource{
			goConstEnum("apps/platform/internal/creator/creation/state.go", "State"),
			goListedConstEnum(
				"apps/platform/internal/creator/creation/state.go", "AllStates",
				"apps/platform/internal/creator/creation/state.go", "State"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "CreationSessionState"),
		},
		absent: "creation_sessions.state carries no CHECK; state.go is the only guard",
	},
	{
		name: "evaluation status",
		sources: []vocabularySource{
			sqlCheckIn("db/migrations/0024_evaluation.sql", "status"),
			goConstEnum("apps/platform/internal/trial/improvement/status.go", "Status"),
			goListedConstEnum(
				"apps/platform/internal/trial/improvement/status.go", "AllStatuses",
				"apps/platform/internal/trial/improvement/status.go", "Status"),
		},
	},
	{
		name: "run attempt object grant state",
		sources: []vocabularySource{
			sqlCheckIn("db/migrations/0050_run_attempt_object_grant_expiry.sql", "object_grants_state"),
			goConstEnum("apps/platform/internal/trial/execution/grantstate.go", "ObjectGrantState"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/grantstate.go", "AllObjectGrantStates",
				"apps/platform/internal/trial/execution/grantstate.go", "ObjectGrantState"),
		},
	},
}

func domainVocabularyProblems(root string) []string {
	return reconcileVocabularies(root, domainVocabularies)
}

func reconcileVocabularies(root string, vocabularies []domainVocabulary) []string {
	var problems []string
	for _, vocabulary := range vocabularies {
		readings := map[string]map[string]bool{}
		union := map[string]bool{}
		failed := false
		for _, source := range vocabulary.sources {
			values, err := source.read(root)
			if err != nil {
				problems = append(problems, fmt.Sprintf("domain-vocabulary: %s: %v", vocabulary.name, err))
				failed = true
				continue
			}
			if len(values) == 0 {
				problems = append(problems, fmt.Sprintf(
					"domain-vocabulary: %s: %s declares no value; either the vocabulary moved or this check is now looking at the wrong place",
					vocabulary.name, source.label))
				failed = true
				continue
			}
			readings[source.label] = values
			for value := range values {
				union[value] = true
			}
		}
		if failed || len(readings) < 2 {
			continue
		}
		var values []string
		for value := range union {
			values = append(values, value)
		}
		sort.Strings(values)
		var labels []string
		for label := range readings {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, value := range values {
			for _, label := range labels {
				if readings[label][value] {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"domain-vocabulary: %s %q is missing from %s; a closed vocabulary declared in more than one place and reconciled in none is how the same concept ends up meaning two things",
					vocabulary.name, value, label))
			}
		}
	}
	return problems
}

func goConstStrings(path, typeName string) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			declared, ok := value.Type.(*ast.Ident)
			if !ok || declared.Name != typeName {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					return nil, fmt.Errorf("%s: %s has an unreadable value: %w", path, name.Name, err)
				}
				values[name.Name] = unquoted
			}
		}
	}
	return values, nil
}

func goListedIdentifiers(path, listName string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		switch declared := decl.(type) {
		case *ast.GenDecl:
			if declared.Tok != token.VAR {
				continue
			}
			for _, spec := range declared.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || value.Names[0].Name != listName || len(value.Values) != 1 {
					continue
				}
				return compositeIdentifiers(path, listName, value.Values[0])
			}
		case *ast.FuncDecl:
			if declared.Recv != nil || declared.Name.Name != listName || declared.Body == nil || len(declared.Body.List) != 1 {
				continue
			}
			returned, ok := declared.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 {
				continue
			}
			return compositeIdentifiers(path, listName, returned.Results[0])
		}
	}
	return nil, fmt.Errorf("%s declares no %s; either the list moved or this check is now looking at the wrong file", path, listName)
}

func compositeIdentifiers(path, listName string, expression ast.Expr) ([]string, error) {
	composite, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not a list this check can read", path, listName)
	}
	var names []string
	for _, element := range composite.Elts {
		switch identifier := element.(type) {
		case *ast.Ident:
			names = append(names, identifier.Name)
		case *ast.SelectorExpr:
			names = append(names, identifier.Sel.Name)
		default:
			return nil, fmt.Errorf("%s: %s holds an entry this check cannot name", path, listName)
		}
	}
	return names, nil
}

func sqlCheckValues(path, column string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`(?s)CHECK\s*\(\s*%s\s+IN\s*\((.*?)\)`, regexp.QuoteMeta(column)))
	match := pattern.FindStringSubmatch(string(raw))
	if match == nil {
		return nil, fmt.Errorf("%s has no CHECK listing the values of %s; the constraint moved and this comparison has lost its subject", path, column)
	}
	return quotedSQLValues(match[1]), nil
}

func quotedSQLValues(list string) map[string]bool {
	values := map[string]bool{}
	for _, entry := range strings.Split(list, ",") {
		trimmed := strings.Trim(strings.TrimSpace(entry), "'")
		if trimmed != "" {
			values[trimmed] = true
		}
	}
	return values
}

func postgresEnumValues(path, typeName string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`(?s)CREATE TYPE\s+%s\s+AS ENUM\s*\((.*?)\)\s*;`, regexp.QuoteMeta(typeName)))
	match := pattern.FindStringSubmatch(string(raw))
	if match == nil {
		return nil, fmt.Errorf("%s declares no enum %s; the type moved and this comparison has lost its subject", path, typeName)
	}
	return quotedSQLValues(match[1]), nil
}
