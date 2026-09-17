package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	envReadingGoRoots     = []string{"apps/platform/cmd", "apps/platform/internal", "apps/sandbox"}
	envReadingPythonRoots = []string{"apps/llm/src"}
	envVarName            = regexp.MustCompile(`^[A-Z][A-Z0-9]*_[A-Z0-9_]*[A-Z0-9]$`)
	pythonEnvRead         = regexp.MustCompile(`os\.(?:getenv\(|environ\.get\(|environ\[)\s*"([A-Z][A-Z0-9_]*)"`)
)

const envReadFloor = 40

func envDeclaredProblems(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		return []string{fmt.Sprintf("env-declared: %v", err)}
	}
	read, err := envVarsRead(root)
	if err != nil {
		return []string{fmt.Sprintf("env-declared: %v", err)}
	}
	if len(read) < envReadFloor {
		return []string{fmt.Sprintf(
			"env-declared: found only %d environment variables read by the deployed services; there are "+
				"at least %d, so the scan is broken rather than the settings gone", len(read), envReadFloor)}
	}
	listed := map[string]bool{}
	for _, m := range envExampleVar.FindAllStringSubmatch(string(data), -1) {
		listed[m[1]] = true
	}
	var problems []string
	for _, name := range sortedKeys(read) {
		if !listed[name] {
			problems = append(problems, fmt.Sprintf(
				"env-declared: %s reads %s, which .env.example does not list; a deployment built from the "+
					"template never learns the setting exists", read[name], name))
		}
	}
	return problems
}

func envVarsRead(root string) (map[string]string, error) {
	read := map[string]string{}
	var goFiles []*goSource
	for _, dir := range envReadingGoRoots {
		files, err := parseGoSources(root, dir)
		if err != nil {
			return nil, err
		}
		goFiles = append(goFiles, files...)
	}
	for name, where := range goEnvReads(goFiles) {
		read[name] = where
	}
	for _, dir := range envReadingPythonRoots {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".py") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range pythonEnvRead.FindAllStringSubmatch(string(data), -1) {
				if _, seen := read[m[1]]; !seen {
					read[m[1]] = relSlash(root, path)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return read, nil
}

type goSource struct {
	path string
	file *ast.File
}

func parseGoSources(root, dir string) ([]*goSource, error) {
	var sources []*goSource
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		sources = append(sources, &goSource{path: relSlash(root, path), file: file})
		return nil
	})
	return sources, err
}

func goEnvReads(sources []*goSource) map[string]string {
	consts := map[string]string{}
	for _, src := range sources {
		for _, decl := range src.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if value, ok := stringLiteral(vs.Values[i]); ok {
							consts[name.Name] = value
						}
					}
				}
			}
		}
	}

	readers := envReaderFuncs(sources)
	read := map[string]string{}
	for _, src := range sources {
		ast.Inspect(src.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			arg, ok := readers[calleeName(call)]
			if !ok || arg >= len(call.Args) {
				return true
			}
			name, ok := stringLiteral(call.Args[arg])
			if !ok {
				name, ok = consts[identName(call.Args[arg])]
			}
			if _, seen := read[name]; ok && envVarName.MatchString(name) && !seen {
				read[name] = src.path
			}
			return true
		})
	}
	return read
}

func envReaderFuncs(sources []*goSource) map[string]int {
	readers := map[string]int{"os.Getenv": 0, "os.LookupEnv": 0}
	for changed := true; changed; {
		changed = false
		for _, src := range sources {
			for _, decl := range src.file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if _, known := readers[fn.Name.Name]; known {
					continue
				}
				params := paramIndexes(fn)
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					arg, isReader := readers[calleeName(call)]
					if !isReader || arg >= len(call.Args) {
						return true
					}
					if index, isParam := params[identName(call.Args[arg])]; isParam {
						readers[fn.Name.Name] = index
						changed = true
						return false
					}
					return true
				})
			}
		}
	}
	return readers
}

func paramIndexes(fn *ast.FuncDecl) map[string]int {
	params := map[string]int{}
	index := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			index++
			continue
		}
		for _, name := range field.Names {
			params[name.Name] = index
			index++
		}
	}
	return params
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "os" {
			return "os." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

func identName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func relSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
