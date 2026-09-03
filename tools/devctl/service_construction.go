package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// serviceConstructionProblems enforces ADR-032 §5: a bounded context receives
// another context's Service from a composition root; it never constructs one in
// a method. Depguard cannot enforce this because approved Customer-Supplier
// collaborators necessarily may import one another.
func serviceConstructionProblems(root string) []string {
	identities, problems := contextTablePackages(
		filepath.Join(root, "docs", "adr", contextMapADR), "docs/adr/"+contextMapADR)
	base := filepath.Join(root, "apps", "platform", "internal")
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		caller, known := resolveContextPath(filepath.ToSlash(filepath.Dir(rel)), identities)
		if !known || ((caller.ID == "apiserver" || caller.ID == "worker") && filepath.ToSlash(filepath.Dir(rel)) == strings.TrimSuffix(caller.Path, "/*")) {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		foreign := map[string]string{}
		constructors := map[string]map[string]bool{}
		dotForeign := ""
		dotConstructors := map[string]bool{}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			internalPath, ok := strings.CutPrefix(importPath, denyPackagePrefix)
			if !ok {
				continue
			}
			target, known := resolveContextPath(internalPath, identities)
			if !known || target.ID == caller.ID || (target.Kind != architectureCore && target.Kind != architectureSupporting) {
				continue
			}
			importDir := filepath.Join(base, filepath.FromSlash(internalPath))
			alias := packageNameAt(importDir, target.ID)
			serviceConstructors := serviceConstructorNamesAt(importDir)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "." {
				dotForeign = target.ID
				for name := range serviceConstructors {
					dotConstructors[name] = true
				}
				continue
			}
			if alias != "_" {
				foreign[alias] = target.ID
				constructors[alias] = serviceConstructors
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if typeSpec, ok := node.(*ast.TypeSpec); ok {
				target := foreignServiceTarget(typeSpec.Type, foreign, dotForeign)
				if target != "" {
					problems = append(problems, fmt.Sprintf(
						"service-construction: %s:%d context %q defines a local type from %q Service; inject it from the process composition root",
						filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, target))
				}
			}
			if valueSpec, ok := node.(*ast.ValueSpec); ok && valueSpec.Type != nil {
				if target := foreignServiceTarget(valueSpec.Type, foreign, dotForeign); target != "" {
					problems = append(problems, fmt.Sprintf(
						"service-construction: %s:%d context %q declares a local %q Service value; inject a pointer from the process composition root",
						filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, target))
				}
			}
			switch ref := node.(type) {
			case *ast.SelectorExpr:
				if pkg, ok := ref.X.(*ast.Ident); ok && pkg.Obj == nil && constructors[pkg.Name][ref.Sel.Name] {
					if target := foreign[pkg.Name]; target != "" {
						problems = append(problems, fmt.Sprintf(
							"service-construction: %s:%d context %q references %q Service constructor %q; inject the Service from the process composition root",
							filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, target, ref.Sel.Name))
					}
				}
			case *ast.Ident:
				if ref.Obj == nil && dotForeign != "" && dotConstructors[ref.Name] {
					problems = append(problems, fmt.Sprintf(
						"service-construction: %s:%d context %q references %q Service constructor %q; inject the Service from the process composition root",
						filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, dotForeign, ref.Name))
				}
			}
			var typ ast.Expr
			switch node := node.(type) {
			case *ast.CompositeLit:
				typ = unparen(node.Type)
			case *ast.CallExpr:
				fun := unparen(node.Fun)
				if target := foreignServiceTarget(fun, foreign, dotForeign); target != "" {
					problems = append(problems, fmt.Sprintf(
						"service-construction: %s:%d context %q converts a value into %q Service; inject it from the process composition root",
						filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, target))
					return true
				}
				name, ok := fun.(*ast.Ident)
				if !ok || name.Name != "new" || len(node.Args) != 1 {
					return true
				}
				typ = unparen(node.Args[0])
			default:
				return true
			}
			target := foreignCompositeServiceTarget(typ, foreign, dotForeign)
			foreignContext := target != ""
			if foreignContext {
				problems = append(problems, fmt.Sprintf(
					"service-construction: %s:%d context %q constructs %q Service; inject it from the process composition root",
					filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), fset.Position(node.Pos()).Line, caller.ID, target))
			}
			return true
		})
		return nil
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("service-construction: apps/platform/internal: %v", err))
	}
	return problems
}

func foreignCompositeServiceTarget(expr ast.Expr, foreign map[string]string, dotForeign string) string {
	if target := foreignServiceTarget(expr, foreign, dotForeign); target != "" {
		return target
	}
	switch typ := unparen(expr).(type) {
	case *ast.StarExpr:
		return foreignCompositeServiceTarget(typ.X, foreign, dotForeign)
	case *ast.ArrayType:
		return foreignCompositeServiceTarget(typ.Elt, foreign, dotForeign)
	case *ast.MapType:
		if target := foreignCompositeServiceTarget(typ.Key, foreign, dotForeign); target != "" {
			return target
		}
		return foreignCompositeServiceTarget(typ.Value, foreign, dotForeign)
	}
	return ""
}

func foreignServiceTarget(expr ast.Expr, foreign map[string]string, dotForeign string) string {
	switch typ := unparen(expr).(type) {
	case *ast.SelectorExpr:
		if typ.Sel.Name == "Service" {
			if pkg, ok := typ.X.(*ast.Ident); ok && pkg.Obj == nil {
				return foreign[pkg.Name]
			}
		}
	case *ast.Ident:
		if typ.Name == "Service" && typ.Obj == nil {
			return dotForeign
		}
	case *ast.ArrayType:
		return foreignServiceTarget(typ.Elt, foreign, dotForeign)
	case *ast.MapType:
		if target := foreignServiceTarget(typ.Key, foreign, dotForeign); target != "" {
			return target
		}
		return foreignServiceTarget(typ.Value, foreign, dotForeign)
	}
	return ""
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func serviceConstructorNamesAt(dir string) map[string]bool {
	constructors := map[string]bool{"NewService": true}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return constructors
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Results == nil {
				continue
			}
			for _, result := range fn.Type.Results.List {
				resultType := unparen(result.Type)
				if pointer, ok := resultType.(*ast.StarExpr); ok {
					resultType = unparen(pointer.X)
				}
				if name, ok := resultType.(*ast.Ident); ok && name.Name == "Service" {
					constructors[fn.Name.Name] = true
				}
			}
		}
	}
	return constructors
}

func packageNameAt(dir, fallback string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fallback
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, parser.PackageClauseOnly)
		if err == nil {
			return file.Name.Name
		}
	}
	return fallback
}
