package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const (
	architectureBudgetFile = "apps/platform/architecture-budget.json"
	platformInternalDir    = "apps/platform/internal"

	maxPackageFiles     = 25
	maxTypeMethods      = 30
	behaviourMethods    = 10
	maxBehaviourFields  = 12
	budgetKindFiles     = "non-test files"
	budgetKindMethods   = "methods"
	budgetKindFields    = "fields"
	budgetGeneratedPart = "gen"
)

type architectureBudget struct {
	Packages map[string]int            `json:"packages"`
	Types    map[string]map[string]int `json:"types"`
}

type budgetMeasure struct {
	files   map[string]int
	methods map[string]int
	fields  map[string]int
}

func architectureBudgetProblems(root string) []string {
	var allowed architectureBudget
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(architectureBudgetFile)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	if err := json.Unmarshal(data, &allowed); err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	measured, err := measureArchitecture(filepath.Join(root, filepath.FromSlash(platformInternalDir)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", platformInternalDir, err)}
	}
	var problems []string
	problems = append(problems, budgetProblems(measured.files, allowed.Packages, maxPackageFiles, budgetKindFiles)...)
	problems = append(problems, budgetProblems(measured.methods, typeBudget(allowed, budgetKindMethods), maxTypeMethods, budgetKindMethods)...)
	behaviour := map[string]int{}
	for name, fields := range measured.fields {
		if measured.methods[name] > behaviourMethods {
			behaviour[name] = fields
		}
	}
	problems = append(problems, budgetProblems(behaviour, typeBudget(allowed, budgetKindFields), maxBehaviourFields, budgetKindFields)...)
	return problems
}

func typeBudget(allowed architectureBudget, kind string) map[string]int {
	out := map[string]int{}
	for name, kinds := range allowed.Types {
		if n, ok := kinds[kind]; ok {
			out[name] = n
		}
	}
	return out
}

func budgetProblems(measured, baseline map[string]int, limit int, kind string) []string {
	var problems []string
	for _, name := range sortedKeys(measured) {
		got := measured[name]
		held, debt := baseline[name]
		switch {
		case got <= limit && debt:
			problems = append(problems, fmt.Sprintf(
				"%s: %s has %d %s, within the limit of %d; remove its entry from %s", architectureBudgetFile, name, got, kind, limit, architectureBudgetFile))
		case got <= limit:
		case !debt:
			problems = append(problems, fmt.Sprintf(
				"%s has %d %s, over the limit of %d; split it by what changes together instead of adding to it", name, got, kind, limit))
		case got > held:
			problems = append(problems, fmt.Sprintf(
				"%s grew to %d %s; %s holds it at %d and only lets it shrink", name, got, kind, architectureBudgetFile, held))
		case got < held:
			problems = append(problems, fmt.Sprintf(
				"%s: %s now has %d %s; lower its entry from %d to %d so it cannot grow back", architectureBudgetFile, name, got, kind, held, got))
		}
	}
	for _, name := range sortedKeys(baseline) {
		if _, present := measured[name]; !present {
			problems = append(problems, fmt.Sprintf(
				"%s: %s no longer exists or no longer counts for %s; remove its entry", architectureBudgetFile, name, kind))
		}
	}
	return problems
}

func measureArchitecture(internal string) (budgetMeasure, error) {
	m := budgetMeasure{files: map[string]int{}, methods: map[string]int{}, fields: map[string]int{}}
	err := filepath.WalkDir(internal, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		if generatedPackage(pkg) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		m.files[pkg]++
		m.record(pkg, file)
		return nil
	})
	return m, err
}

func generatedPackage(pkg string) bool {
	for _, part := range strings.Split(pkg, "/") {
		if part == budgetGeneratedPart {
			return true
		}
	}
	return false
}

func (m budgetMeasure) record(pkg string, file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			m.recordMethod(pkg, d)
		case *ast.GenDecl:
			m.recordStructs(pkg, d)
		}
	}
}

func (m budgetMeasure) recordMethod(pkg string, fn *ast.FuncDecl) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return
	}
	if name, ok := receiverTypeName(fn.Recv.List[0].Type); ok {
		m.methods[pkg+"."+name]++
	}
}

func (m budgetMeasure) recordStructs(pkg string, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		if ts, ok := spec.(*ast.TypeSpec); ok {
			if st, ok := ts.Type.(*ast.StructType); ok {
				m.fields[pkg+"."+ts.Name.Name] = structFieldCount(st)
			}
		}
	}
}

func structFieldCount(st *ast.StructType) int {
	count := 0
	for _, field := range st.Fields.List {
		count += max(1, len(field.Names))
	}
	return count
}
