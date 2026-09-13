package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const identifierOrderMinDeclarations = 140

var identifierOrderWorkspaceNames = map[string]bool{
	"workspace":   true,
	"workspaceid": true,
	"ws":          true,
	"wsid":        true,
}

func identifierOrderProblems(root string) []string {
	var problems []string
	seen := 0
	for _, base := range []string{
		filepath.Join(root, "apps", "platform", "internal"),
		filepath.Join(root, "apps", "platform", "cmd"),
	} {
		walkErr := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
			fileProblems, fileSeen, err := identifierOrderFileProblems(path, relative)
			if err != nil {
				return err
			}
			problems = append(problems, fileProblems...)
			seen += fileSeen
			return nil
		})
		if walkErr != nil {
			problems = append(problems, fmt.Sprintf("identifier-order: %s: %v", base, walkErr))
		}
	}
	if seen < identifierOrderMinDeclarations {
		problems = append(problems, fmt.Sprintf(
			"identifier-order: only found %d declaration(s) with a pgtype.UUID parameter (want at least %d); "+
				"the checker is likely pointed at the wrong path", seen, identifierOrderMinDeclarations))
	}
	return problems
}

func identifierOrderFileProblems(path, relative string) ([]string, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, 0, err
	}
	var problems []string
	seen := 0
	report := func(pos token.Pos, format string, args ...any) {
		location := fmt.Sprintf("%s:%d", relative, fset.Position(pos).Line)
		problems = append(problems, fmt.Sprintf("identifier-order: %s: %s", location, fmt.Sprintf(format, args...)))
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			checkNestedParamNames(d.Type, d.Name.Name, report)
			params := flattenIdentifierParams(d.Type.Params)
			if countPgtypeUUIDParams(params) == 0 {
				continue
			}
			seen++
			checkWorkspaceOrder(params, d.Name.Name, d.Pos(), report)
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch t := typeSpec.Type.(type) {
				case *ast.FuncType:
					checkNestedParamNames(t, typeSpec.Name.Name, report)
					params := flattenIdentifierParams(t.Params)
					if countPgtypeUUIDParams(params) == 0 {
						continue
					}
					seen++
					checkWorkspaceOrder(params, typeSpec.Name.Name, typeSpec.Pos(), report)
				case *ast.InterfaceType:
					for _, method := range t.Methods.List {
						if len(method.Names) == 0 {
							continue
						}
						checkNestedParamNames(method.Type, method.Names[0].Name, report)
						methodType, ok := method.Type.(*ast.FuncType)
						if !ok {
							continue
						}
						params := flattenIdentifierParams(methodType.Params)
						if countPgtypeUUIDParams(params) == 0 {
							continue
						}
						seen++
						checkWorkspaceOrder(params, method.Names[0].Name, method.Pos(), report)
					}
				case *ast.StructType:
					for _, field := range t.Fields.List {
						if len(field.Names) == 0 {
							continue
						}
						checkNestedParamNames(field.Type, field.Names[0].Name, report)
						fieldType, ok := field.Type.(*ast.FuncType)
						if !ok {
							continue
						}
						params := flattenIdentifierParams(fieldType.Params)
						if countPgtypeUUIDParams(params) == 0 {
							continue
						}
						seen++
						checkWorkspaceOrder(params, field.Names[0].Name, field.Pos(), report)
					}
				}
			}
		}
	}
	return problems, seen, nil
}

type identifierParam struct {
	name        string
	isPgUUID    bool
	isWorkspace bool
}

func flattenIdentifierParams(fields *ast.FieldList) []identifierParam {
	if fields == nil {
		return nil
	}
	var params []identifierParam
	for _, field := range fields.List {
		isPgUUID := isPgtypeUUID(field.Type)
		if len(field.Names) == 0 {
			params = append(params, identifierParam{isPgUUID: isPgUUID})
			continue
		}
		for _, name := range field.Names {
			params = append(params, identifierParam{
				name:        name.Name,
				isPgUUID:    isPgUUID,
				isWorkspace: isPgUUID && identifierOrderWorkspaceNames[strings.ToLower(name.Name)],
			})
		}
	}
	return params
}

func isPgtypeUUID(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "pgtype" && sel.Sel.Name == "UUID"
}

func countPgtypeUUIDParams(params []identifierParam) int {
	count := 0
	for _, p := range params {
		if p.isPgUUID {
			count++
		}
	}
	return count
}

func checkWorkspaceOrder(params []identifierParam, subject string, pos token.Pos, report func(token.Pos, string, ...any)) {
	firstWorkspace := -1
	for i, p := range params {
		if p.isWorkspace {
			firstWorkspace = i
			break
		}
	}
	if firstWorkspace <= 0 {
		return
	}
	for i := 0; i < firstWorkspace; i++ {
		if params[i].isPgUUID {
			report(pos, "%s has a pgtype.UUID parameter ahead of its workspace identifier; the workspace parameter must come first", subject)
			return
		}
	}
}

func checkNestedParamNames(node ast.Node, subject string, report func(token.Pos, string, ...any)) {
	ast.Inspect(node, func(n ast.Node) bool {
		if funcType, ok := n.(*ast.FuncType); ok {
			checkParamNames(flattenIdentifierParams(funcType.Params), subject, funcType.Pos(), report)
		}
		return true
	})
}

func checkParamNames(params []identifierParam, subject string, pos token.Pos, report func(token.Pos, string, ...any)) {
	if countPgtypeUUIDParams(params) < 2 {
		return
	}
	for _, p := range params {
		if p.isPgUUID && p.name == "" {
			report(pos, "%s has two or more unnamed pgtype.UUID parameters; name every parameter", subject)
			return
		}
	}
}
