package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIdentifierOrderSample(t *testing.T, root, body string) {
	t.Helper()
	writeGoFile(t, root, "apps/platform/internal/sample/sample.go", body)
	if err := os.MkdirAll(filepath.Join(root, "apps", "platform", "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestIdentifierOrderWorkspaceFirstPasses(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import "github.com/jackc/pgx/v5/pgtype"

func WorkspaceSkill(workspaceID, skillID pgtype.UUID) {}
`)
	if problems := identifierOrderProblems(root); containsPrefix(problems, "identifier-order: apps/platform/internal/sample/sample.go") {
		t.Fatalf("workspace-first signature was flagged: %#v", problems)
	}
}

func TestIdentifierOrderWorkspaceAfterAnotherUUIDFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import "github.com/jackc/pgx/v5/pgtype"

func WorkspaceSkill(skillID, workspaceID pgtype.UUID) {}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "sample.go:5", "WorkspaceSkill", "workspace parameter must come first") {
		t.Fatalf("expected a workspace-order violation for WorkspaceSkill, got %#v", problems)
	}
}

func TestIdentifierOrderSingleUUIDPasses(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import "github.com/jackc/pgx/v5/pgtype"

func CatalogSkill(skillID pgtype.UUID) {}
`)
	if problems := identifierOrderProblems(root); containsPrefix(problems, "identifier-order: apps/platform/internal/sample/sample.go") {
		t.Fatalf("a single identifier parameter was flagged: %#v", problems)
	}
}

func TestIdentifierOrderIdentityWorkspaceTypeIsNotCounted(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

func ListDatasets(ws identity.Workspace, testCaseID pgtype.UUID) {}
`)
	if problems := identifierOrderProblems(root); containsPrefix(problems, "identifier-order: apps/platform/internal/sample/sample.go") {
		t.Fatalf("an identity.Workspace parameter was treated as a pgtype.UUID: %#v", problems)
	}
}

func TestIdentifierOrderMultiNameFieldFlattensInOrder(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import "context"
import "github.com/jackc/pgx/v5/pgtype"

func Ledger(ctx context.Context, subject, workspaceID pgtype.UUID) {}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "Ledger", "workspace parameter must come first") {
		t.Fatalf("a workspace identifier sharing a field with an earlier name was not flagged: %#v", problems)
	}
}

func TestIdentifierOrderStructFuncFieldTwoUnnamedUUIDsFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	ReadSkill func(context.Context, pgtype.UUID, pgtype.UUID) (int, error)
}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "ReadSkill", "unnamed pgtype.UUID parameters") {
		t.Fatalf("two unnamed pgtype.UUID parameters on a struct func field were not flagged: %#v", problems)
	}
}

func TestIdentifierOrderStructFuncFieldSingleUnnamedUUIDPasses(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	ReadCompatibility func(context.Context, pgtype.UUID) (int, error)
}
`)
	if problems := identifierOrderProblems(root); containsPrefix(problems, "identifier-order: apps/platform/internal/sample/sample.go") {
		t.Fatalf("a single unnamed pgtype.UUID parameter was flagged: %#v", problems)
	}
}

func TestIdentifierOrderNamedFuncTypeTwoUnnamedUUIDsFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Reader func(context.Context, pgtype.UUID, pgtype.UUID) (int, error)
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "Reader", "unnamed pgtype.UUID parameters") {
		t.Fatalf("a named func type with two unnamed pgtype.UUID parameters was not flagged: %#v", problems)
	}
}

func TestIdentifierOrderInterfaceMethodOrderViolationFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Reader interface {
	WorkspaceSkill(ctx context.Context, skillID, workspaceID pgtype.UUID) (int, error)
}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "WorkspaceSkill", "workspace parameter must come first") {
		t.Fatalf("an interface method with the workspace parameter out of order was not flagged: %#v", problems)
	}
}

func TestIdentifierOrderInterfaceMethodTwoUnnamedUUIDsFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Reader interface {
	ReadSkill(context.Context, pgtype.UUID, pgtype.UUID) (int, error)
}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "sample.go:10", "ReadSkill", "unnamed pgtype.UUID parameters") {
		t.Fatalf("an interface method with two unnamed pgtype.UUID parameters was not flagged: %#v", problems)
	}
}

func TestIdentifierOrderFuncTypeInASignatureTwoUnnamedUUIDsFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderSample(t, root, `package sample

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func readContentSource() func(context.Context, pgtype.UUID, pgtype.UUID) (int, error) {
	return nil
}
`)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "sample.go:9", "readContentSource", "unnamed pgtype.UUID parameters") {
		t.Fatalf("a function type returned from a signature was not held to the naming rule: %#v", problems)
	}
}

func TestIdentifierOrderSkipsTestFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps", "platform", "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoFile(t, root, "apps/platform/internal/sample/sample_test.go", `package sample

import "github.com/jackc/pgx/v5/pgtype"

func brokenHelper(skillID, workspaceID pgtype.UUID) {}
`)
	if problems := identifierOrderProblems(root); containsPrefix(problems, "identifier-order: apps/platform/internal/sample/sample_test.go") {
		t.Fatalf("a _test.go file was not skipped: %#v", problems)
	}
}

func TestIdentifierOrderSkipsGeneratedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps", "platform", "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoFile(t, root, genDirRelative+"/broken.go", `package gen

import "github.com/jackc/pgx/v5/pgtype"

func BrokenHelper(skillID, workspaceID pgtype.UUID) {}
`)
	for _, problem := range identifierOrderProblems(root) {
		if strings.Contains(problem, genDirRelative) {
			t.Fatalf("a generated file was not skipped: %#v", problem)
		}
	}
}

func writeIdentifierOrderFuncs(t *testing.T, root string, count int) {
	t.Helper()
	var body strings.Builder
	body.WriteString("package sample\n\nimport \"github.com/jackc/pgx/v5/pgtype\"\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&body, "func F%d(workspaceID pgtype.UUID) {}\n", i)
	}
	if err := os.MkdirAll(filepath.Join(root, "apps", "platform", "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoFile(t, root, "apps/platform/internal/sample/sample.go", body.String())
}

func TestIdentifierOrderSelfCheckAtThresholdPasses(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderFuncs(t, root, identifierOrderMinDeclarations)
	for _, problem := range identifierOrderProblems(root) {
		if strings.Contains(problem, "pointed at the wrong path") {
			t.Fatalf("the self-check fired exactly at the threshold: %#v", problem)
		}
	}
}

func TestIdentifierOrderSelfCheckJustBelowThresholdFails(t *testing.T) {
	root := t.TempDir()
	writeIdentifierOrderFuncs(t, root, identifierOrderMinDeclarations-1)
	problems := identifierOrderProblems(root)
	if !containsAll(problems, "pointed at the wrong path") {
		t.Fatalf("the self-check did not fire one declaration below the threshold: %#v", problems)
	}
}

func TestTheRealRepositoryHasNoIdentifierOrderProblems(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := identifierOrderProblems(root); len(problems) > 0 {
		t.Fatalf("identifier parameters are out of order or unnamed:\n%s", strings.Join(problems, "\n"))
	}
}

func containsAll(problems []string, substrings ...string) bool {
	joined := strings.Join(problems, "\n")
	for _, s := range substrings {
		if !strings.Contains(joined, s) {
			return false
		}
	}
	return true
}

func containsPrefix(problems []string, prefix string) bool {
	for _, p := range problems {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
