package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// genDirRelative is the one and only data layer 02:PORT-008 permits.
const genDirRelative = "apps/platform/internal/foundation/persistence/db/gen"

// secondDataLayerMinHits is how many identically-named, identically-typed
// methods a type needs before it counts as a replica rather than
// coincidence. coder/coder's dbmem — the shape this check exists to catch —
// stood in for the whole Querier interface, hundreds of methods deep; three
// is already far past what an unrelated type would share by accident.
const secondDataLayerMinHits = 3

// secondDataLayerProblems flags a Go type outside db/gen that declares
// secondDataLayerMinHits or more methods matching a db/gen method both by
// name and by full, qualifier-stripped parameter/result signature.
//
// 02:PORT-008 says no second data layer, but until this check existed nothing
// enforced it. db/sqlc.yaml does not set emit_interface, so there is no
// Querier type in this repo — a fake implementation has no relationship to
// *gen.Queries that the Go compiler will ever notice, and a struct with the
// right method names compiles clean either way. coder/coder shipped exactly
// this shape as dbmem and deleted it in 2024-11 (PR #15291) once the fake and
// real Postgres had drifted far enough that only the fake's own tests still
// trusted it.
//
// The match is on signature, not name, on purpose: apps/platform/internal/
// trial/design.Service declares four methods named like gen queries
// (CreateTestCase, GetTestCase, ListTestCases, UpdateTestCase) and a
// name-only check would flag it immediately. It is not a replica — it takes
// identity.Workspace where gen takes *Params types — and a name-only match
// cannot tell the two apart. A type that could actually stand in for
// *gen.Queries has to accept gen's Params/Row types, because nothing else
// compiles against gen's callers; that is what the signature check verifies.
//
// What this does NOT catch: a second data layer that also renames its
// methods — a `memStore` with `getSkillByID`/`putRun` instead of
// `GetSkillByID`/`PutRun`. This check starts from db/gen's own method names
// and looks for lookalikes; a renamed replica shares no name with anything
// it scans for and is invisible to it. This is a tripwire against the
// specific shape coder/coder shipped, not a proof that no second data layer
// exists.
func secondDataLayerProblems(root string) []string {
	genSigs, err := genMethodSignatures(root)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", genDirRelative, err)}
	}
	if len(genSigs) == 0 {
		return []string{fmt.Sprintf("%s: found no methods to compare against; second-data-layer check cannot run", genDirRelative)}
	}

	hitsByType := map[string][]sigHit{}
	for _, base := range []string{"apps", "tools", "packages"} {
		dir := filepath.Join(root, base)
		if _, err := os.Stat(dir); err != nil {
			continue // fixtures do not have to carry all three roots.
		}
		walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if strings.HasPrefix(relative, genDirRelative+"/") {
				return nil // db/gen is the data layer, not a candidate second one.
			}
			return collectSecondDataLayerHits(path, relative, genSigs, hitsByType)
		})
		if walkErr != nil {
			return []string{fmt.Sprintf("%s: %v", base, walkErr)}
		}
	}

	var problems []string
	for _, key := range sortedKeys(hitsByType) {
		hits := hitsByType[key]
		if len(hits) < secondDataLayerMinHits {
			continue
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].method < hits[j].method })
		methods := make([]string, 0, len(hits))
		for _, h := range hits {
			methods = append(methods, fmt.Sprintf("%s (%s)", h.method, h.location))
		}
		problems = append(problems, fmt.Sprintf(
			"second data layer suspected (02:PORT-008): %s declares %d method(s) matching %s by name and signature: %s",
			key, len(hits), genDirRelative, strings.Join(methods, ", ")))
	}
	return problems
}

// sigHit is one method on a candidate type whose name and normalized
// signature matched a db/gen method.
type sigHit struct {
	method   string
	location string
}

// genMethodSignatures reads every generated query file (*.sql.go — db.go and
// models.go carry no query methods and are excluded on purpose) and returns
// each method name mapped to its normalized signature text.
func genMethodSignatures(root string) (map[string]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(genDirRelative))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sigs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			sigs[fn.Name.Name] = normalizeFuncType(fn.Type)
		}
	}
	return sigs, nil
}

// collectSecondDataLayerHits parses one Go file and records, for every
// method whose name and normalized signature match genSigs, a hit keyed by
// the method's package directory and receiver type — so methods on the same
// type declared across multiple files in one package still count together.
func collectSecondDataLayerHits(path, relative string, genSigs map[string]string, hitsByType map[string][]sigHit) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil // a file that fails to parse fails the build, not this check.
	}
	dir := filepath.ToSlash(filepath.Dir(relative))
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		typeName, ok := receiverTypeName(fn.Recv.List[0].Type)
		if !ok {
			continue
		}
		wantSig, known := genSigs[fn.Name.Name]
		if !known || normalizeFuncType(fn.Type) != wantSig {
			continue
		}
		key := dir + "." + typeName
		hitsByType[key] = append(hitsByType[key], sigHit{
			method:   fn.Name.Name,
			location: fmt.Sprintf("%s:%d", relative, fset.Position(fn.Pos()).Line),
		})
	}
	return nil
}

// receiverTypeName returns the base identifier of a method receiver type,
// unwrapping the pointer if there is one. Generic receivers (Foo[T]) are not
// used anywhere in this repo's persistence code and are reported as unknown
// rather than guessed at.
func receiverTypeName(expr ast.Expr) (string, bool) {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// normalizeFuncType renders a function signature as a canonical, comparable
// string: parameter and result types only (names dropped, since a replica
// naming its argument `params` instead of `arg` is still a replica), with
// every package qualifier stripped (so gen.CreateUserParams and
// CreateUserParams — the same type, referenced from inside and outside
// package gen — compare equal).
//
// The clone builds entirely fresh nodes with no source positions, rather
// than mutating the parsed tree in place. A tree that mixes real positions
// from the parsed file with the zero position of a freshly built identifier
// confuses go/printer's line-break heuristics — it was observed to emit a
// stray trailing comma inside an otherwise single-line parameter list — so
// the safe way to get one deterministic, position-free rendering is to never
// hand printer a mixed one.
func normalizeFuncType(ft *ast.FuncType) string {
	clone := &ast.FuncType{
		Params:  cloneFieldListTypes(ft.Params),
		Results: cloneFieldListTypes(ft.Results),
	}
	var buf bytes.Buffer
	// Printing can only fail by write error, and bytes.Buffer never returns
	// one; the node itself is one we built, so it always has legal shape.
	_ = printer.Fprint(&buf, token.NewFileSet(), clone)
	return buf.String()
}

func cloneFieldListTypes(fl *ast.FieldList) *ast.FieldList {
	out := &ast.FieldList{}
	if fl == nil {
		return out
	}
	for _, field := range fl.List {
		count := len(field.Names)
		if count == 0 {
			count = 1 // an unnamed field is still one parameter or result.
		}
		for i := 0; i < count; i++ {
			out.List = append(out.List, &ast.Field{Type: cloneTypeExpr(field.Type)})
		}
	}
	return out
}

// cloneTypeExpr rebuilds a type expression with fresh, position-free nodes
// and every package qualifier (pkg.Type) collapsed to its bare type name.
// It covers the type shapes db/gen's generated signatures actually use —
// identifiers, pointers, slices, variadics, and maps. Anything else (channel
// types, nested func types, generic instantiations) is returned unchanged:
// none of those appear in sqlc output, and falling back to the original node
// only risks under-matching a signature, never over-matching one.
func cloneTypeExpr(expr ast.Expr) ast.Expr {
	switch x := expr.(type) {
	case *ast.Ident:
		return ast.NewIdent(x.Name)
	case *ast.SelectorExpr:
		return ast.NewIdent(x.Sel.Name)
	case *ast.StarExpr:
		return &ast.StarExpr{X: cloneTypeExpr(x.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Len: x.Len, Elt: cloneTypeExpr(x.Elt)}
	case *ast.Ellipsis:
		return &ast.Ellipsis{Elt: cloneTypeExpr(x.Elt)}
	case *ast.MapType:
		return &ast.MapType{Key: cloneTypeExpr(x.Key), Value: cloneTypeExpr(x.Value)}
	default:
		return expr
	}
}
