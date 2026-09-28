package main

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

const genImportPath = "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"

type callSite struct {
	boundary string
	caller   string
	path     string
}

func queryCallSites(platform string, names map[string]bool, identities map[string]packageIdentity) (map[string][]callSite, error) {
	calls := map[string][]callSite{}
	for _, tree := range []string{"internal", platformCmdDir} {
		source := platformSourceTree{name: tree, base: filepath.Join(platform, tree)}
		if _, err := os.Stat(source.base); err != nil {
			continue
		}
		err := filepath.WalkDir(source.base, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			if !isScannedPlatformSource(path) {
				return nil
			}
			return addFileQueryCallSites(calls, source, path, names, identities)
		})
		if err != nil {
			return nil, err
		}
	}
	return calls, nil
}

func isScannedPlatformSource(path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	normalizedPath := filepath.ToSlash(path)
	for _, skipped := range rawSQLSkippedDirs {
		if strings.Contains(normalizedPath, "/"+skipped+"/") {
			return false
		}
	}
	return true
}

type platformSourceTree struct {
	name string
	base string
}

func addFileQueryCallSites(calls map[string][]callSite, tree platformSourceTree, path string,
	names map[string]bool, identities map[string]packageIdentity) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	seen, interfaceQueries := referencedQueryNames(file, names)
	if len(seen) == 0 {
		return nil
	}
	relative, err := filepath.Rel(tree.base, path)
	if err != nil {
		return err
	}
	relative = filepath.ToSlash(relative)
	directory := filepath.ToSlash(filepath.Dir(relative))
	boundary, known := callerBoundary(tree.name, directory, identities)
	if !known {
		return unknownQueryCallerError(tree.name, directory)
	}

	for name := range seen {
		siteBoundary, caller := boundary, boundary
		if interfaceQueries[name] {
			siteBoundary, caller = "", "a query-shaped interface"
		}
		calls[name] = append(calls[name], callSite{
			boundary: siteBoundary,
			caller:   caller,
			path:     "apps/platform/" + tree.name + "/" + relative,
		})
	}
	return nil
}

func referencedQueryNames(file *ast.File, names map[string]bool) (seen, interfaceQueries map[string]bool) {
	importsGen := importsSQLCPackage(file)
	seen = map[string]bool{}
	interfaceQueries = map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			if importsGen && names[node.Sel.Name] {
				seen[node.Sel.Name] = true
			}
		case *ast.InterfaceType:
			for _, method := range node.Methods.List {
				for _, name := range method.Names {
					if names[name.Name] {
						seen[name.Name] = true
						interfaceQueries[name.Name] = true
					}
				}
			}
		}
		return true
	})
	return seen, interfaceQueries
}

func importsSQLCPackage(file *ast.File) bool {
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err == nil && importPath == genImportPath {
			return true
		}
	}
	return false
}

func unknownQueryCallerError(tree, directory string) error {
	if tree == platformCmdDir {
		return fmt.Errorf("apps/platform/cmd/%s calls sqlc but has no entry in commandContexts "+
			"(tools/devctl/query_owners.go); name the context whose data it touches", directory)
	}
	return fmt.Errorf("apps/platform/internal/%s calls sqlc but has no architecture identity in %s", directory, identityHomes)
}

func callerBoundary(tree, directory string, identities map[string]packageIdentity) (string, bool) {
	if tree == platformCmdDir {
		command, _, _ := strings.Cut(directory, "/")
		boundary, ok := commandContexts[command]
		if !ok || !knownBoundaryID(identities, boundary) {
			return "", false
		}
		return boundary, true
	}
	identity, known := resolveContextPath(directory, identities)
	return identity.ID, known
}

var rawSQLEntryPoints = map[string]bool{
	"Exec": true, "Query": true, "QueryRow": true, "Queue": true,
}

var rawSQLKeywordPattern = regexp.MustCompile(`\b(?:SELECT|INSERT|UPDATE|DELETE|SET|CREATE|ALTER|DROP|TRUNCATE|WITH)\b`)

var rawSQLSkippedDirs = []string{
	"apps/platform/internal/foundation/persistence/db/gen",
	"apps/platform/internal/entrypoint/api/gen",
}

type rawSQLSite struct {
	line     int
	function string
	method   string
	sql      string
}

func rawSQLCallSites(path string) ([]rawSQLSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	packageStrings := packageConstStrings(file)
	var sites []rawSQLSite
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		sites = append(sites, functionRawSQLSites(fset, fn, packageStrings)...)
	}
	return sites, nil
}

func packageConstStrings(file *ast.File) map[string]string {
	packageStrings := map[string]string{}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.CONST {
			bindValueSpecs(gen.Specs, packageStrings)
		}
	}
	return packageStrings
}

func functionRawSQLSites(fset *token.FileSet, fn *ast.FuncDecl, packageStrings map[string]string) []rawSQLSite {
	stringsByName := make(map[string]string, len(packageStrings))
	for name, value := range packageStrings {
		stringsByName[name] = value
	}
	bindLocalStrings(fn.Body, stringsByName)
	var sites []rawSQLSite
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !rawSQLEntryPoints[selector.Sel.Name] {
			return true
		}
		arg, text, found := firstStringArgument(call, stringsByName)
		if found && rawSQLKeywordPattern.MatchString(normalizeSQL(text)) {
			sites = append(sites, rawSQLSite{
				line: fset.Position(arg.Pos()).Line, function: fn.Name.Name,
				method: selector.Sel.Name, sql: sqlPrefix(text),
			})
		}
		return true
	})
	return sites
}

func firstStringArgument(call *ast.CallExpr, stringsByName map[string]string) (ast.Expr, string, bool) {
	for _, arg := range call.Args {
		if text, ok := stringValue(arg, stringsByName); ok {
			return arg, text, true
		}
	}
	return nil, "", false
}

func bindLocalStrings(body *ast.BlockStmt, values map[string]string) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.DeclStmt:
			if gen, ok := node.Decl.(*ast.GenDecl); ok {
				bindValueSpecs(gen.Specs, values)
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if i >= len(node.Rhs) {
					break
				}
				name, ok := lhs.(*ast.Ident)
				if value, found := stringValue(node.Rhs[i], values); ok && found {
					values[name.Name] = value
				}
			}
		}
		return true
	})
}

func bindValueSpecs(specs []ast.Spec, values map[string]string) {
	for _, spec := range specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				break
			}
			if value, ok := stringValue(valueSpec.Values[i], values); ok {
				values[name.Name] = value
			}
		}
	}
}

func stringValue(expr ast.Expr, values map[string]string) (string, bool) {
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if expr.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expr.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := values[expr.Name]
		return value, ok
	case *ast.ParenExpr:
		return stringValue(expr.X, values)
	case *ast.BinaryExpr:
		if expr.Op != token.ADD {
			return "", false
		}
		left, leftOK := stringValue(expr.X, values)
		right, rightOK := stringValue(expr.Y, values)
		return left + " " + right, leftOK || rightOK
	case *ast.CallExpr:
		selector, ok := expr.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		pkg, pkgOK := selector.X.(*ast.Ident)
		if !pkgOK || pkg.Name != "fmt" || selector.Sel.Name != "Sprintf" || len(expr.Args) == 0 {
			return "", false
		}
		return stringValue(expr.Args[0], values)
	default:
		return "", false
	}
}

const sqlPrefixMaxChars = 60

func sqlPrefix(sql string) string {
	flat := strings.Join(strings.Fields(sql), " ")
	if len(flat) > sqlPrefixMaxChars {
		return flat[:sqlPrefixMaxChars] + "…"
	}
	return flat
}
