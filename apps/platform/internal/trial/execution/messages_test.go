package run

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

func TestTheSentencesTheRunHandlerWritesAreInTheInterfaceLanguage(t *testing.T) {
	for label, msg := range map[string]string{
		"messageCreditBalance":           messageCreditBalance,
		"messagePreflightTargetNotFound": messagePreflightTargetNotFound,
	} {
		if !hasHan(msg) {
			t.Errorf("%s: %q is what a reader sees, so it belongs in the interface language", label, msg)
		}
	}
	if got := notFoundMessage(ErrPreflightTargetNotFound); got != messagePreflightTargetNotFound {
		t.Errorf("a missing preflight target is reported as %q", got)
	}
	if got := notFoundMessage(ErrNotFound); got != ErrNotFound.Error() {
		t.Errorf("a missing run is reported as %q, want its own message", got)
	}
}

func TestNoDomainErrorInThisContextCarriesInterfaceCopy(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				fn, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || fn.Sel.Name != "New" {
					return true
				}
				if pkgName, ok := fn.X.(*ast.Ident); !ok || pkgName.Name != "errors" {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok {
					return true
				}
				message, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				read++
				if hasHan(message) {
					t.Errorf("%s: %q is interface copy living in a domain error; "+
						"the sentence a reader sees belongs to the handler that writes it",
						fset.Position(call.Pos()), message)
				}
				return true
			})
		}
	}
	if read == 0 {
		t.Fatal("found no errors.New in this package; this test read nothing")
	}
}
