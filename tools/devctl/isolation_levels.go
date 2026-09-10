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

const isolationGoFile = "apps/platform/internal/trial/execution/schedule.go"

// Each entry names the key the enum hangs under, since sandbox-provider
// nests it under "isolation:" while public.yaml uses a flat
// "isolation_level:".
var isolationContractFiles = []struct{ path, marker string }{
	{"contracts/openapi/sandbox-provider.yaml", "isolation:"},
	{"contracts/openapi/public.yaml", "isolation_level:"},
}

// Anchored so a mention inside a description string can't be mistaken for
// the schema key.
func isolationMarkerPattern(marker string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(marker) + `\s*$`)
}

// Gate isolation constants are named <value>Isolation, so a new one joins
// the comparison just by being declared.
var isolationConstSuffix = "Isolation"

// Reads "enum: [a, b, c]" directly rather than through a YAML parser.
var isolationEnumPattern = regexp.MustCompile(`(?m)^\s*enum: \[([a-z, ]+)\]\s*$`)

func isolationLevelProblems(root string) []string {
	declared, err := gateIsolationLevels(filepath.Join(root, isolationGoFile))
	if err != nil {
		return []string{fmt.Sprintf("isolation levels: %v", err)}
	}
	if len(declared) == 0 {
		return []string{fmt.Sprintf("isolation levels: %s declares no *%s constant; either the gate moved or this check is now looking at the wrong file",
			isolationGoFile, isolationConstSuffix)}
	}
	var problems []string
	for _, contract := range isolationContractFiles {
		admitted, err := contractIsolationEnum(filepath.Join(root, filepath.FromSlash(contract.path)), contract.marker)
		if err != nil {
			problems = append(problems, fmt.Sprintf("isolation levels: %v", err))
			continue
		}
		var names []string
		for name := range declared {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if admitted[declared[name]] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"isolation level %q (%s in %s) is not in %s's %s enum; a dispatch gate that accepts a level the contract does not admit is a capability nothing validates",
				declared[name], name, isolationGoFile, contract.path, strings.TrimSuffix(contract.marker, ":")))
		}
	}
	return problems
}

// Reads constant values via the AST rather than grepping text, so a comment
// mentioning a value can't be mistaken for its declaration.
func gateIsolationLevels(path string) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	levels := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasSuffix(name.Name, isolationConstSuffix) || i >= len(value.Values) {
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
				levels[name.Name] = unquoted
			}
		}
	}
	return levels, nil
}

func contractIsolationEnum(path, marker string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	at := isolationMarkerPattern(marker).FindAllStringIndex(text, -1)
	switch len(at) {
	case 1:
	case 0:
		return nil, fmt.Errorf("%s has no `%s` schema key; the field moved and this comparison has lost its subject", path, marker)
	default:
		return nil, fmt.Errorf("%s declares `%s` %d times; this check would be picking a winner between two schemas", path, marker, len(at))
	}
	match := isolationEnumPattern.FindStringSubmatch(text[at[0][0]:])
	if match == nil {
		return nil, fmt.Errorf(
			"%s: no enum found under `%s`; a prose list of levels is not something a checker can read, "+
				"and that is exactly how `clean` was emitted here for a day with nothing noticing (2026-08-29)",
			path, marker)
	}
	admitted := map[string]bool{}
	for _, value := range strings.Split(match[1], ",") {
		if v := strings.TrimSpace(value); v != "" {
			admitted[v] = true
		}
	}
	return admitted, nil
}
