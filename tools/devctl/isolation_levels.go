package main

// The dispatch gate's allow list and the provider contract's isolation enum are
// two statements of the same set, written in two languages, and nothing was
// comparing them.
//
// That is not hypothetical. `clean` was added to the gate on 2026-08-28 and to
// contracts/openapi/sandbox-provider.yaml only on 2026-08-29, and in between the
// system emitted a capability value the contract did not admit. Nothing failed,
// because this provider serves its capability from hand-written structs rather
// than from generated types - so the usual drift check, which regenerates and
// diffs, has nothing to look at here. A contract that only describes the code by
// coincidence is the shape this repository keeps recording.
//
// What this checks is one direction, and deliberately so: every level the gate
// can accept must be spelled in the contract. The reverse is allowed - the enum
// may name a level (`vm`) that no deployment of ours accepts yet, because the
// contract describes what a provider may say, not what we will take.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	isolationGoFile       = "apps/platform/internal/trial/execution/schedule.go"
	isolationContractFile = "contracts/openapi/sandbox-provider.yaml"
)

// isolationConstSuffix is what makes a constant part of the set rather than a
// list this checker has to be told about: the gate names its levels
// <something>Isolation, so a fourth one joins the comparison by being declared,
// not by someone remembering to add it here.
var isolationConstSuffix = "Isolation"

// isolationEnumPattern reads the enum off the contract without a YAML parser:
// the line is `enum: [gvisor, container, vm, process, clean]` under
// isolation.level, and pulling it out by shape keeps this checker free of a
// dependency whose absence is the only reason tools/devctl builds anywhere.
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
	admitted, err := contractIsolationEnum(filepath.Join(root, isolationContractFile))
	if err != nil {
		return []string{fmt.Sprintf("isolation levels: %v", err)}
	}
	var problems []string
	for name, level := range declared {
		if !admitted[level] {
			problems = append(problems, fmt.Sprintf(
				"isolation level %q (%s in %s) is not in %s's isolation.level enum; a dispatch gate that accepts a level the contract does not admit is a capability nothing validates",
				level, name, isolationGoFile, isolationContractFile))
		}
	}
	return problems
}

// gateIsolationLevels reads the constant values from the AST rather than from
// the file's text, for the reason require_db_guard.go records: a checker that
// greps finds its own subject matter in the comments around it and passes a
// mutation that removed the code.
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

func contractIsolationEnum(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	marker := strings.Index(text, "isolation:")
	if marker < 0 {
		return nil, fmt.Errorf("%s has no isolation schema", path)
	}
	match := isolationEnumPattern.FindStringSubmatch(text[marker:])
	if match == nil {
		return nil, fmt.Errorf("%s: no isolation.level enum found after the isolation schema", path)
	}
	admitted := map[string]bool{}
	for _, value := range strings.Split(match[1], ",") {
		if v := strings.TrimSpace(value); v != "" {
			admitted[v] = true
		}
	}
	return admitted, nil
}
