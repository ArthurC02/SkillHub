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

func serviceConstructionProblems(root string) []string {
	identities, problems := architectureIdentities(root)
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
		if !known || (isCompositionRoot(caller.ID) && filepath.ToSlash(filepath.Dir(rel)) == strings.TrimSuffix(caller.Path, "/*")) {
			return nil
		}
		fileProblems, err := serviceConstructionFileProblems(root, base, path, caller, identities)
		if err != nil {
			return err
		}
		problems = append(problems, fileProblems...)
		return nil
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("service-construction: apps/platform/internal: %v", err))
	}
	return problems
}

type foreignServiceImports struct {
	byAlias         map[string]string
	constructors    map[string]map[string]bool
	dotTarget       string
	dotConstructors map[string]bool
}

func serviceConstructionFileProblems(root, base, path string, caller packageIdentity,
	identities map[string]packageIdentity) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	imports := foreignServiceImportsOf(file, base, caller.ID, identities)
	shownPath := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
	var problems []string
	ast.Inspect(file, func(node ast.Node) bool {
		if finding := imports.constructionFinding(node); finding != "" {
			problems = append(problems, fmt.Sprintf("service-construction: %s:%d context %q %s",
				shownPath, fset.Position(node.Pos()).Line, caller.ID, finding))
		}
		return true
	})
	return problems, nil
}

func foreignServiceImportsOf(file *ast.File, base, callerID string, identities map[string]packageIdentity) foreignServiceImports {
	imports := foreignServiceImports{
		byAlias:         map[string]string{},
		constructors:    map[string]map[string]bool{},
		dotConstructors: map[string]bool{},
	}
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
		if !known || target.ID == callerID || (target.Kind != architectureCore && target.Kind != architectureSupporting) {
			continue
		}
		importDir := filepath.Join(base, filepath.FromSlash(internalPath))
		alias := packageNameAt(importDir, target.ID)
		serviceConstructors := serviceConstructorNamesAt(importDir)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias == "." {
			imports.dotTarget = target.ID
			for name := range serviceConstructors {
				imports.dotConstructors[name] = true
			}
			continue
		}
		if alias != "_" {
			imports.byAlias[alias] = target.ID
			imports.constructors[alias] = serviceConstructors
		}
	}
	return imports
}

func (imports foreignServiceImports) constructionFinding(node ast.Node) string {
	switch node := node.(type) {
	case *ast.TypeSpec:
		if target := foreignServiceTarget(node.Type, imports.byAlias, imports.dotTarget); target != "" {
			return fmt.Sprintf("defines a local type from %q Service; inject it from the process composition root", target)
		}
	case *ast.ValueSpec:
		if node.Type == nil {
			return ""
		}
		if target := foreignServiceTarget(node.Type, imports.byAlias, imports.dotTarget); target != "" {
			return fmt.Sprintf("declares a local %q Service value; inject a pointer from the process composition root", target)
		}
	case *ast.SelectorExpr:
		return imports.qualifiedConstructorFinding(node)
	case *ast.Ident:
		if node.Obj == nil && imports.dotTarget != "" && imports.dotConstructors[node.Name] {
			return constructorReferenceFinding(imports.dotTarget, node.Name)
		}
	case *ast.CompositeLit:
		return imports.compositeConstructionFinding(unparen(node.Type))
	case *ast.CallExpr:
		return imports.callConstructionFinding(node)
	}
	return ""
}

func (imports foreignServiceImports) qualifiedConstructorFinding(ref *ast.SelectorExpr) string {
	pkg, ok := ref.X.(*ast.Ident)
	if !ok || pkg.Obj != nil || !imports.constructors[pkg.Name][ref.Sel.Name] {
		return ""
	}
	if target := imports.byAlias[pkg.Name]; target != "" {
		return constructorReferenceFinding(target, ref.Sel.Name)
	}
	return ""
}

func constructorReferenceFinding(target, constructor string) string {
	return fmt.Sprintf("references %q Service constructor %q; inject the Service from the process composition root", target, constructor)
}

func (imports foreignServiceImports) callConstructionFinding(call *ast.CallExpr) string {
	fun := unparen(call.Fun)
	if target := foreignServiceTarget(fun, imports.byAlias, imports.dotTarget); target != "" {
		return fmt.Sprintf("converts a value into %q Service; inject it from the process composition root", target)
	}
	name, ok := fun.(*ast.Ident)
	if !ok || name.Name != "new" || len(call.Args) != 1 {
		return ""
	}
	return imports.compositeConstructionFinding(unparen(call.Args[0]))
}

func (imports foreignServiceImports) compositeConstructionFinding(typ ast.Expr) string {
	if target := foreignCompositeServiceTarget(typ, imports.byAlias, imports.dotTarget); target != "" {
		return fmt.Sprintf("constructs %q Service; inject it from the process composition root", target)
	}
	return ""
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
			if ok && fn.Recv == nil && returnsService(fn.Type.Results) {
				constructors[fn.Name.Name] = true
			}
		}
	}
	return constructors
}

func returnsService(results *ast.FieldList) bool {
	if results == nil {
		return false
	}
	for _, result := range results.List {
		resultType := unparen(result.Type)
		if pointer, ok := resultType.(*ast.StarExpr); ok {
			resultType = unparen(pointer.X)
		}
		if name, ok := resultType.(*ast.Ident); ok && name.Name == "Service" {
			return true
		}
	}
	return false
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
