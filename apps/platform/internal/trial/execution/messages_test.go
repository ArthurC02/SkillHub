package run

import (
	"go/ast"
	"go/token"
	"strconv"
	"testing"
)

func TestTheSentencesTheRunHandlerWritesAreInTheInterfaceLanguage(t *testing.T) {
	for label, msg := range map[string]string{
		"messageCreditBalance":           messageCreditBalance,
		"messagePreflightTargetNotFound": messagePreflightTargetNotFound,
		"messageRunNotFound":             messageRunNotFound,
	} {
		if !hasHan(msg) {
			t.Errorf("%s: %q is what a reader sees, so it belongs in the interface language", label, msg)
		}
	}
	if got := notFoundMessage(ErrPreflightTargetNotFound); got != messagePreflightTargetNotFound {
		t.Errorf("a missing preflight target is reported as %q", got)
	}
	if got := notFoundMessage(ErrNotFound); got != messageRunNotFound {
		t.Errorf("a missing run is reported as %q, want the handler's own sentence", got)
	}
}

func inspectErrorsNewCall(t *testing.T, fset *token.FileSet, n ast.Node) bool {
	t.Helper()
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	fn, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || fn.Sel.Name != "New" {
		return false
	}
	if pkgName, ok := fn.X.(*ast.Ident); !ok || pkgName.Name != "errors" {
		return false
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok {
		return false
	}
	message, err := strconv.Unquote(literal.Value)
	if err != nil {
		return false
	}
	if hasHan(message) {
		t.Errorf("%s: %q is interface copy living in a domain error; "+
			"the sentence a reader sees belongs to the handler that writes it",
			fset.Position(call.Pos()), message)
	}
	return true
}

func TestNoDomainErrorInThisContextCarriesInterfaceCopy(t *testing.T) {
	fset, files := packageSourceFiles(t)
	read := 0
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if inspectErrorsNewCall(t, fset, n) {
				read++
			}
			return true
		})
	}
	if read == 0 {
		t.Fatal("found no errors.New in this package; this test read nothing")
	}
}
