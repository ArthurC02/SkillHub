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
	"sort"
	"strconv"
	"strings"
)

const isolationGoFile = "apps/platform/internal/trial/execution/schedule.go"

// EVERY contract that spells the set, not just the first one found.
//
// The first version of this check compared the gate against
// sandbox-provider.yaml alone, and the audit of 2026-08-29 found the same drift
// still open one file over: public.yaml's RunPermissionSummaryContent carried
// `isolation_level` as a free string whose description listed four levels in
// PROSE — no `clean`. That field is what TEST-008's preflight screen shows a
// user before their run starts ("how strongly will this be isolated"), so the
// contract that was wrong is the one facing outward, and a prose list is
// something no checker can read. The enum landed in 331bd90; this is what keeps
// it a set rather than a sentence.
//
// Each entry names the key the enum hangs under, because the two contracts spell
// the same field differently: sandbox-provider has `isolation: { level: … }`,
// public.yaml has a flat `isolation_level:`.
var isolationContractFiles = []struct{ path, marker string }{
	{"contracts/openapi/sandbox-provider.yaml", "isolation:"},
	{"contracts/openapi/public.yaml", "isolation_level:"},
}

// The key line, anchored so that a mention inside a description cannot be
// mistaken for the schema key. Exactly one match is required per file: zero
// means the field moved, more than one means this check would be picking a
// winner between two schemas.
func isolationMarkerPattern(marker string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(marker) + `\s*$`)
}

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
