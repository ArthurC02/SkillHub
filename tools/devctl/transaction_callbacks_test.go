package main

import (
	"strings"
	"testing"
)

func transactionFixture(t *testing.T, held, source string) []string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, architectureBudgetFile, `{"packages": {}, "types": {}, "transaction_callbacks": [`+held+`]}`)
	writeAt(t, root, platformInternalDir+"/ctx/a/a.go", "package a\n\nimport \"github.com/jackc/pgx/v5\"\n\n"+source)
	return transactionCallbackProblems(root)
}

func TestEveryExportedShapeThatHandsOutATransactionIsFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, source, want string
	}{
		{"a struct field", "type Params struct {\n\tSameTransaction func(pgx.Tx) error\n}\n", "ctx/a.Params.SameTransaction (ctx/a/a.go:6)"},
		{"a named func type", "type Hook func(ctx int, tx pgx.Tx) error\n", "ctx/a.Hook (ctx/a/a.go:5)"},
		{"a method parameter", "type S struct{}\n\nfunc (s *S) Do(fn func(pgx.Tx) error) {}\n", "ctx/a.S.Do (ctx/a/a.go:7)"},
		{"a function parameter", "func Do(fn func(pgx.Tx) error) {}\n", "ctx/a.Do (ctx/a/a.go:5)"},
	}
	for _, c := range cases {
		p := transactionFixture(t, "", c.source)
		if len(p) != 1 || !strings.Contains(p[0], c.want) || !strings.Contains(p[0], "hands a transaction") {
			t.Errorf("%s: %v, want %q named", c.name, p, c.want)
		}
	}
}

func TestShapesThatDoNotHandATransactionAcrossAreIgnored(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, source string
	}{
		{"an unexported field", "type Params struct {\n\tsame func(pgx.Tx) error\n}\n"},
		{"an unexported func type", "type hook func(pgx.Tx) error\n"},
		{"an unexported function", "func do(fn func(pgx.Tx) error) {}\n"},
		{"a transaction passed as a value", "func Do(tx pgx.Tx) {}\n"},
		{"a callback over another package's Tx", "func Do(fn func(other.Tx) error) {}\n"},
	}
	for _, c := range cases {
		if p := transactionFixture(t, "", c.source); len(p) != 0 {
			t.Errorf("%s: %v, want none", c.name, p)
		}
	}
}

func TestATransactionCallbackInTheBaselineIsHeldAndAStaleOneMustBeRemoved(t *testing.T) {
	t.Parallel()
	source := "func Do(fn func(pgx.Tx) error) {}\n"
	if p := transactionFixture(t, `"ctx/a.Do"`, source); len(p) != 0 {
		t.Errorf("a held callback: %v, want none", p)
	}
	p := transactionFixture(t, `"ctx/a.Do", "ctx/a.Gone"`, source)
	if len(p) != 1 || !strings.Contains(p[0], "ctx/a.Gone no longer hands out a transaction") {
		t.Errorf("a stale entry: %v, want it named for removal", p)
	}
}

func TestAGenericPackageMayCallItsOwnerBackWithItsTransaction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, architectureBudgetFile, `{"packages": {}, "types": {}}`)
	hook := "package a\n\nimport \"github.com/jackc/pgx/v5\"\n\ntype MarkFunc func(pgx.Tx) error\n"
	writeAt(t, root, platformInternalDir+"/foundation/storage/a/a.go", hook)
	writeAt(t, root, platformInternalDir+"/foundationlike/a/a.go", hook)
	p := transactionCallbackProblems(root)
	if len(p) != 1 || !strings.Contains(p[0], "foundationlike/a.MarkFunc") {
		t.Errorf("problems = %v, want only the package outside foundation named", p)
	}
}

func TestTestFilesMayHandOutTransactions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, architectureBudgetFile, `{"packages": {}, "types": {}}`)
	writeAt(t, root, platformInternalDir+"/ctx/a/a_test.go", "package a\n\nimport \"github.com/jackc/pgx/v5\"\n\nfunc Do(fn func(pgx.Tx) error) {}\n")
	if p := transactionCallbackProblems(root); len(p) != 0 {
		t.Errorf("a test file was counted: %v", p)
	}
}

func TestTheRepositoryHandsOutNoNewTransactions(t *testing.T) {
	t.Parallel()
	if p := transactionCallbackProblems("../.."); len(p) != 0 {
		t.Errorf("transaction callbacks: %v", p)
	}
}
