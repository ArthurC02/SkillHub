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

type transactionCallbackBaseline struct {
	TransactionCallbacks []string `json:"transaction_callbacks"`
}

func transactionCallbackProblems(root string) []string {
	var baseline transactionCallbackBaseline
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(architectureBudgetFile)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	found, err := transactionCallbacks(filepath.Join(root, filepath.FromSlash(platformInternalDir)))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", platformInternalDir, err)}
	}
	held := map[string]bool{}
	for _, name := range baseline.TransactionCallbacks {
		held[name] = true
	}
	var problems []string
	for _, name := range sortedKeys(found) {
		if !held[name] {
			problems = append(problems, fmt.Sprintf(
				"%s (%s) hands a transaction to code the package does not own; let the owner of the table "+
					"write it, or make the call idempotent and link afterwards", name, found[name]))
		}
	}
	for _, name := range sortedKeys(held) {
		if _, present := found[name]; !present {
			problems = append(problems, fmt.Sprintf(
				"%s: %s no longer hands out a transaction; remove it from transaction_callbacks", architectureBudgetFile, name))
		}
	}
	return problems
}

func transactionCallbacks(internal string) (map[string]string, error) {
	found := map[string]string{}
	err := filepath.WalkDir(internal, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(internal, path)
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if generatedPackage(pkg) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for name, pos := range exportedTransactionCallbacks(file) {
			found[pkg+"."+name] = fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(pos).Line)
		}
		return nil
	})
	return found, err
}

func exportedTransactionCallbacks(file *ast.File) map[string]token.Pos {
	found := map[string]token.Pos{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					typeTransactionCallbacks(ts, found)
				}
			}
		case *ast.FuncDecl:
			if d.Name.IsExported() && anyTakesTransaction(d.Type.Params.List) {
				found[funcDeclName(d)] = d.Pos()
			}
		}
	}
	return found
}

func typeTransactionCallbacks(ts *ast.TypeSpec, found map[string]token.Pos) {
	if takesTransaction(ts.Type) {
		found[ts.Name.Name] = ts.Pos()
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.IsExported() && takesTransaction(field.Type) {
				found[ts.Name.Name+"."+name.Name] = name.Pos()
			}
		}
	}
}

func anyTakesTransaction(params []*ast.Field) bool {
	for _, param := range params {
		if takesTransaction(param.Type) {
			return true
		}
	}
	return false
}

func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		if name, ok := receiverTypeName(fn.Recv.List[0].Type); ok {
			return name + "." + fn.Name.Name
		}
	}
	return fn.Name.Name
}

func takesTransaction(expr ast.Expr) bool {
	fn, ok := expr.(*ast.FuncType)
	if !ok || fn.Params == nil {
		return false
	}
	for _, param := range fn.Params.List {
		if sel, ok := param.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Tx" {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "pgx" {
				return true
			}
		}
	}
	return false
}
