package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const genDirRelative = "apps/platform/internal/foundation/persistence/db/gen"

const secondDataLayerMinHits = 3

func secondDataLayerProblems(root string) []string {
	genSigs, err := genMethodSignatures(root)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", genDirRelative, err)}
	}
	if len(genSigs) == 0 {
		return []string{fmt.Sprintf("%s: found no methods to compare against; second-data-layer check cannot run", genDirRelative)}
	}

	hitsByType := map[string][]sigHit{}
	for _, base := range []string{"apps", "tools", "packages"} {
		dir := filepath.Join(root, base)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if strings.HasPrefix(relative, genDirRelative+"/") {
				return nil
			}
			return collectSecondDataLayerHits(path, relative, genSigs, hitsByType)
		})
		if walkErr != nil {
			return []string{fmt.Sprintf("%s: %v", base, walkErr)}
		}
	}

	var problems []string
	for _, key := range sortedKeys(hitsByType) {
		hits := hitsByType[key]
		if len(hits) < secondDataLayerMinHits {
			continue
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].method < hits[j].method })
		methods := make([]string, 0, len(hits))
		for _, h := range hits {
			methods = append(methods, fmt.Sprintf("%s (%s)", h.method, h.location))
		}
		problems = append(problems, fmt.Sprintf(
			"second data layer suspected (02:PORT-008): %s declares %d method(s) matching %s by name and signature: %s",
			key, len(hits), genDirRelative, strings.Join(methods, ", ")))
	}
	return problems
}

type sigHit struct {
	method   string
	location string
}

func genMethodSignatures(root string) (map[string]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(genDirRelative))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sigs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			sigs[fn.Name.Name] = normalizeFuncType(fn.Type)
		}
	}
	return sigs, nil
}

func collectSecondDataLayerHits(path, relative string, genSigs map[string]string, hitsByType map[string][]sigHit) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(relative))
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		typeName, ok := receiverTypeName(fn.Recv.List[0].Type)
		if !ok {
			continue
		}
		wantSig, known := genSigs[fn.Name.Name]
		if !known || normalizeFuncType(fn.Type) != wantSig {
			continue
		}
		key := dir + "." + typeName
		hitsByType[key] = append(hitsByType[key], sigHit{
			method:   fn.Name.Name,
			location: fmt.Sprintf("%s:%d", relative, fset.Position(fn.Pos()).Line),
		})
	}
	return nil
}

func receiverTypeName(expr ast.Expr) (string, bool) {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// normalizeFuncType prints a rebuilt, position-free copy of ft: printing the
// original node mixes real and synthetic positions and can mangle formatting.
func normalizeFuncType(ft *ast.FuncType) string {
	clone := &ast.FuncType{
		Params:  cloneFieldListTypes(ft.Params),
		Results: cloneFieldListTypes(ft.Results),
	}
	var buf bytes.Buffer

	_ = printer.Fprint(&buf, token.NewFileSet(), clone)
	return buf.String()
}

func cloneFieldListTypes(fl *ast.FieldList) *ast.FieldList {
	out := &ast.FieldList{}
	if fl == nil {
		return out
	}
	for _, field := range fl.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			out.List = append(out.List, &ast.Field{Type: cloneTypeExpr(field.Type)})
		}
	}
	return out
}

func cloneTypeExpr(expr ast.Expr) ast.Expr {
	switch x := expr.(type) {
	case *ast.Ident:
		return ast.NewIdent(x.Name)
	case *ast.SelectorExpr:
		return ast.NewIdent(x.Sel.Name)
	case *ast.StarExpr:
		return &ast.StarExpr{X: cloneTypeExpr(x.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Len: x.Len, Elt: cloneTypeExpr(x.Elt)}
	case *ast.Ellipsis:
		return &ast.Ellipsis{Elt: cloneTypeExpr(x.Elt)}
	case *ast.MapType:
		return &ast.MapType{Key: cloneTypeExpr(x.Key), Value: cloneTypeExpr(x.Value)}
	default:
		return expr
	}
}
