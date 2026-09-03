package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceConstructionADR = `### 1. Context 對照表

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| Evaluation | Core | eval | trial/improvement | EVAL |
| Run Trace | Supporting | trace | trial/evidence | TRACE |
| — | Generic | apiserver | entrypoint/api/apiserver | — |
| — | Generic | api | entrypoint/api/gen | — |
| — | Generic | worker | entrypoint/worker | — |
`

func writeServiceConstructionFixture(t *testing.T, relative, source string) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range map[string]string{
		"docs/adr/" + contextMapADR:                    serviceConstructionADR,
		"apps/platform/internal/" + relative + "/x.go": source,
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestServiceConstructionRejectsForeignServiceLiterals(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, expression, want string }{
		{"composite literal", "&trace.Service{}", `context "eval" constructs "trace" Service`},
		{"new builtin", "new(trace.Service)", `context "eval" constructs "trace" Service`},
		{"parenthesized new builtin", "new((trace.Service))", `context "eval" constructs "trace" Service`},
		{"constructor", "trace.NewService()", `context "eval" references "trace" Service constructor "NewService"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := writeServiceConstructionFixture(t, "trial/improvement", `package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = `+test.expression+` }
`)
			problems := strings.Join(serviceConstructionProblems(root), "\n")
			if !strings.Contains(problems, test.want) {
				t.Fatalf("foreign Service construction was accepted: %s", problems)
			}
		})
	}
}

func TestServiceConstructionUsesTheImportedPackagesActualName(t *testing.T) {
	t.Parallel()
	root := writeServiceConstructionFixture(t, "trial/improvement", `package eval
import "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence/reader"
func f() { _ = &reader.Service{} }
`)
	target := filepath.Join(root, "apps", "platform", "internal", "trial", "evidence", "reader", "reader.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package reader\ntype Service struct{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	problems := strings.Join(serviceConstructionProblems(root), "\n")
	if !strings.Contains(problems, `context "eval" constructs "trace" Service`) {
		t.Fatalf("nested package Service construction was accepted: %s", problems)
	}
}

func TestServiceConstructionRejectsAliasAndDotImportBypasses(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { type traceService = (trace.Service); _ = &traceService{} }
`,
		`package eval
import . "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = &Service{} }
`,
	} {
		root := writeServiceConstructionFixture(t, "trial/improvement", source)
		if problems := serviceConstructionProblems(root); len(problems) == 0 {
			t.Fatal("foreign Service construction through an alias was accepted")
		}
	}

	root := writeServiceConstructionFixture(t, "trial/improvement", `package eval
import . "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = Event{} }
`)
	if problems := serviceConstructionProblems(root); len(problems) != 0 {
		t.Fatalf("a dot-import without Service construction was rejected: %v", problems)
	}
}

func TestServiceConstructionRejectsDefinedTypesConversionsAndValues(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { var local trace.Service; _ = local }
`,
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
type localService trace.Service
func f() { _ = trace.Service(localService{}) }
`,
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = []trace.Service{{}} }
`,
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = map[string]trace.Service{"x": {}} }
`,
		`package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = []*trace.Service{{}} }
`,
	} {
		root := writeServiceConstructionFixture(t, "trial/improvement", source)
		if problems := serviceConstructionProblems(root); len(problems) == 0 {
			t.Fatal("a foreign Service value construction was accepted")
		}
	}
}

func TestServiceConstructionFindsOwnerNamedConstructors(t *testing.T) {
	t.Parallel()
	root := writeServiceConstructionFixture(t, "trial/improvement", `package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = trace.New() }
`)
	target := filepath.Join(root, "apps", "platform", "internal", "trial", "evidence", "service.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package trace\ntype Service struct{}\nfunc New() *Service { return &Service{} }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	problems := strings.Join(serviceConstructionProblems(root), "\n")
	if !strings.Contains(problems, `references "trace" Service constructor "New"`) {
		t.Fatalf("owner-named Service constructor was accepted: %s", problems)
	}
}

func TestServiceConstructionRejectsConstructorMethodValues(t *testing.T) {
	t.Parallel()
	root := writeServiceConstructionFixture(t, "trial/improvement", `package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { constructor := trace.NewService; _ = constructor }
`)
	problems := strings.Join(serviceConstructionProblems(root), "\n")
	if !strings.Contains(problems, `references "trace" Service constructor "NewService"`) {
		t.Fatalf("foreign Service constructor method value was accepted: %s", problems)
	}
}

func TestServiceConstructionDoesNotExemptGeneratedAPIChildren(t *testing.T) {
	t.Parallel()
	root := writeServiceConstructionFixture(t, "entrypoint/api/gen", `package gen
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = &trace.Service{} }
`)
	if problems := serviceConstructionProblems(root); len(problems) == 0 {
		t.Fatal("a generated API package was treated as a composition root")
	}
}

func TestServiceConstructionAllowsCompositionRootsAndOtherTypes(t *testing.T) {
	t.Parallel()
	root := writeServiceConstructionFixture(t, "entrypoint/api/apiserver", `package apiserver
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = &trace.Service{}; _ = trace.Event{} }
`)
	if problems := serviceConstructionProblems(root); len(problems) != 0 {
		t.Fatalf("composition-root construction was rejected: %v", problems)
	}

	root = writeServiceConstructionFixture(t, "trial/improvement", `package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { _ = trace.Event{} }
`)
	if problems := serviceConstructionProblems(root); len(problems) != 0 {
		t.Fatalf("a foreign non-Service value was rejected: %v", problems)
	}

	root = writeServiceConstructionFixture(t, "trial/improvement", `package eval
import trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
func f() { trace := struct{ Service struct{} }{}; _ = trace.Service{} }
`)
	if problems := serviceConstructionProblems(root); len(problems) != 0 {
		t.Fatalf("a local value shadowing an import alias was rejected: %v", problems)
	}
}
