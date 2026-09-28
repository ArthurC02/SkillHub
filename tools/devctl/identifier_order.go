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
		filepath.Join(root, "apps", "platform", platformCmdDir),
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
			seen += checkIdentifierSignature(d.Type, d.Name.Name, d.Pos(), report)
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				seen += checkTypeDeclSignatures(d, report)
			}
		}
	}
	return problems, seen, nil
}

func checkTypeDeclSignatures(decl *ast.GenDecl, report func(token.Pos, string, ...any)) int {
	seen := 0
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		switch t := typeSpec.Type.(type) {
		case *ast.FuncType:
			seen += checkIdentifierSignature(t, typeSpec.Name.Name, typeSpec.Pos(), report)
		case *ast.InterfaceType:
			seen += checkNamedFieldSignatures(t.Methods, report)
		case *ast.StructType:
			seen += checkNamedFieldSignatures(t.Fields, report)
		}
	}
	return seen
}

func checkNamedFieldSignatures(fields *ast.FieldList, report func(token.Pos, string, ...any)) int {
	seen := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			continue
		}
		seen += checkIdentifierSignature(field.Type, field.Names[0].Name, field.Pos(), report)
	}
	return seen
}

func checkIdentifierSignature(node ast.Node, subject string, pos token.Pos, report func(token.Pos, string, ...any)) (uuidSignaturesSeen int) {
	checkNestedParamNames(node, subject, report)
	funcType, ok := node.(*ast.FuncType)
	if !ok {
		return 0
	}
	params := flattenIdentifierParams(funcType.Params)
	if countPgtypeUUIDParams(params) == 0 {
		return 0
	}
	checkWorkspaceOrder(params, subject, pos, report)
	return 1
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
