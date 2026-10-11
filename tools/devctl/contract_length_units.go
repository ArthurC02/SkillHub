package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const contractLimitsImport = "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"

func contractLengthUnitProblems(root string) []string {
	internal := filepath.Join(root, filepath.FromSlash(platformInternalDir))
	packages, err := parsedPlatformPackages(internal)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", platformInternalDir, err)}
	}
	var problems []string
	for _, files := range packages {
		aliases := characterLimitAliases(files)
		for _, f := range files {
			problems = append(problems, byteCountedCharacterLimits(f, aliases)...)
		}
	}
	sort.Strings(problems)
	return problems
}

type parsedGoFile struct {
	rel  string
	fset *token.FileSet
	file *ast.File
}

func parsedPlatformPackages(internal string) (map[string][]parsedGoFile, error) {
	packages := map[string][]parsedGoFile{}
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
		packages[pkg] = append(packages[pkg], parsedGoFile{rel: filepath.ToSlash(rel), fset: fset, file: file})
		return nil
	})
	return packages, err
}

func contractLimitsName(file *ast.File) string {
	for _, spec := range file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path == contractLimitsImport {
			if spec.Name != nil {
				return spec.Name.Name
			}
			return "gen"
		}
	}
	return ""
}

func characterLimit(expr ast.Expr, limitsName string) (string, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || limitsName == "" {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != limitsName || !strings.HasSuffix(sel.Sel.Name, "MaxLength") {
		return "", false
	}
	return sel.Sel.Name, true
}

func characterLimitAliases(files []parsedGoFile) map[string]string {
	aliases := map[string]string{}
	for _, f := range files {
		limitsName := contractLimitsName(f.file)
		ast.Inspect(f.file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, value := range spec.Values {
				if limit, ok := characterLimit(value, limitsName); ok && i < len(spec.Names) {
					aliases[spec.Names[i].Name] = limit
				}
			}
			return true
		})
	}
	return aliases
}

func byteCountedCharacterLimits(f parsedGoFile, aliases map[string]string) []string {
	limitsName := contractLimitsName(f.file)
	limitOf := func(expr ast.Expr) (string, bool) {
		if limit, ok := characterLimit(expr, limitsName); ok {
			return limit, true
		}
		if ident, ok := expr.(*ast.Ident); ok {
			limit, ok := aliases[ident.Name]
			return limit, ok
		}
		return "", false
	}
	var problems []string
	ast.Inspect(f.file, func(n ast.Node) bool {
		cmp, ok := n.(*ast.BinaryExpr)
		if !ok || !isOrdering(cmp.Op) {
			return true
		}
		for _, pair := range [][2]ast.Expr{{cmp.X, cmp.Y}, {cmp.Y, cmp.X}} {
			if !isLenCall(pair[0]) {
				continue
			}
			if limit, ok := limitOf(pair[1]); ok {
				problems = append(problems, fmt.Sprintf(
					"%s:%d compares len(), a byte count, with the contract's %s, which counts characters; "+
						"use utf8.RuneCountInString, or declare the field x-max-bytes in the contract",
					f.rel, f.fset.Position(cmp.Pos()).Line, limit))
			}
		}
		return true
	})
	return problems
}

func isOrdering(op token.Token) bool {
	return op == token.GTR || op == token.GEQ || op == token.LSS || op == token.LEQ
}

func isLenCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == "len"
}
