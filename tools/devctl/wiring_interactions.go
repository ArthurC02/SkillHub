package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	platformModuleDir    = "apps/platform"
	platformInternalPath = "github.com/ArthurC02/skillhub/apps/platform/internal/"
	interactionsFile     = "docs/domain-memory/registry/interactions.json"
	registryReviewed     = "reviewed"
)

var compositionRootPatterns = []string{"./internal/entrypoint/...", "./cmd/..."}

type contextEdge struct{ from, to string }

func (e contextEdge) String() string { return e.from + "->" + e.to }

type listedPackage struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
	DepOnly    bool
}

func wiringInteractionProblems(root string) []string {
	identities, problems := architectureIdentities(root)
	if len(problems) > 0 {
		return problems
	}
	permitted, _, problems := reviewedDependencyPermissions(root, nil)
	if len(problems) > 0 {
		return problems
	}
	interactions, err := reviewedInteractions(root)
	if err != nil {
		return []string{err.Error()}
	}
	var baseline struct {
		WiringEdges []string `json:"wiring_edges"`
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(architectureBudgetFile)))
	if err == nil {
		err = json.Unmarshal(data, &baseline)
	}
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	contextOf := func(importPath string) (string, bool) {
		rel, ok := strings.CutPrefix(importPath, platformInternalPath)
		if !ok {
			return "", false
		}
		identity, ok := resolveContextPath(rel, identities)
		if !ok || (identity.Kind != architectureCore && identity.Kind != architectureSupporting) {
			return "", false
		}
		return identity.ID, true
	}
	moduleDir, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(platformModuleDir)))
	if err != nil {
		return []string{fmt.Sprintf("wiring-interactions: %v", err)}
	}
	edges, err := wiringEdges(moduleDir, compositionRootPatterns, contextOf)
	if err != nil {
		return []string{fmt.Sprintf("wiring-interactions: %v", err)}
	}
	registered := func(e contextEdge) bool { return permitted[e.from][e.to] || interactions[e] }
	return unregisteredEdgeProblems(edges, registered, baseline.WiringEdges)
}

func unregisteredEdgeProblems(edges map[contextEdge]string, registered func(contextEdge) bool, baseline []string) []string {
	held := map[string]bool{}
	for _, name := range baseline {
		held[name] = true
	}
	found := map[string]string{}
	for edge, at := range edges {
		if !registered(edge) {
			found[edge.String()] = at
		}
	}
	var problems []string
	for _, name := range sortedKeys(found) {
		if !held[name] {
			problems = append(problems, fmt.Sprintf(
				"%s (%s) wires one Context into another with no reviewed dependency policy or interaction in the Registry; "+
					"record the collaboration through Domain Memory before wiring it", name, found[name]))
		}
	}
	for _, name := range sortedKeys(held) {
		if _, present := found[name]; !present {
			problems = append(problems, fmt.Sprintf(
				"%s: %s is registered or no longer wired; remove it from wiring_edges", architectureBudgetFile, name))
		}
	}
	return problems
}

func reviewedInteractions(root string) (map[contextEdge]bool, error) {
	data, err := registryAsset(root, "interactions")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", interactionsFile, err)
	}
	var doc struct {
		Interactions []struct {
			Consumer string `json:"consumer_context"`
			Producer string `json:"producer_context"`
			Status   string `json:"status"`
		} `json:"interactions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", interactionsFile, err)
	}
	out := map[contextEdge]bool{}
	for _, i := range doc.Interactions {
		if i.Status == registryReviewed {
			out[contextEdge{i.Consumer, i.Producer}] = true
		}
	}
	return out, nil
}

func wiringEdges(moduleDir string, patterns []string, contextOf func(string) (string, bool)) (map[contextEdge]string, error) {
	args := append([]string{"list", "-deps", "-export", "-json=ImportPath,Dir,Export,GoFiles,DepOnly"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleDir
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
	}
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	exports := map[string]string{}
	var roots []listedPackage
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, err
		}
		exports[pkg.ImportPath] = pkg.Export
		if !pkg.DepOnly {
			roots = append(roots, pkg)
		}
	}
	edges := map[contextEdge]string{}
	for _, pkg := range roots {
		if err := packageWiringEdges(moduleDir, pkg, exports, contextOf, edges); err != nil {
			return nil, err
		}
	}
	return edges, nil
}

func packageWiringEdges(moduleDir string, pkg listedPackage, exports map[string]string, contextOf func(string) (string, bool), edges map[contextEdge]string) error {
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(pkg.GoFiles))
	for _, name := range pkg.GoFiles {
		file, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files = append(files, file)
	}
	lookup := func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) }
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	if _, err := conf.Check(pkg.ImportPath, fset, files, info); err != nil {
		return fmt.Errorf("type-check %s: %w", pkg.ImportPath, err)
	}
	w := wiringWalker{info: info, contextOf: contextOf, edges: edges, fset: fset, moduleDir: moduleDir}
	for _, file := range files {
		ast.Inspect(file, w.visit)
	}
	return nil
}

type wiringWalker struct {
	info      *types.Info
	contextOf func(string) (string, bool)
	edges     map[contextEdge]string
	fset      *token.FileSet
	moduleDir string
}

func (w wiringWalker) visit(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.CompositeLit:
		w.injectIntoStruct(n)
	case *ast.AssignStmt:
		w.injectIntoFields(n)
	case *ast.CallExpr:
		w.injectIntoParameters(n)
	}
	return true
}

func (w wiringWalker) injectIntoStruct(lit *ast.CompositeLit) {
	owner, ok := w.typeContext(w.info.Types[lit].Type)
	if !ok {
		return
	}
	for _, element := range lit.Elts {
		if kv, ok := element.(*ast.KeyValueExpr); ok {
			element = kv.Value
		}
		w.recordBehaviour(owner, element)
	}
}

func (w wiringWalker) injectIntoFields(assign *ast.AssignStmt) {
	if len(assign.Lhs) != len(assign.Rhs) {
		return
	}
	for i, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if owner, ok := w.typeContext(w.info.Types[sel.X].Type); ok {
			w.recordBehaviour(owner, assign.Rhs[i])
		}
	}
}

func (w wiringWalker) injectIntoParameters(call *ast.CallExpr) {
	owner, ok := w.calleeContext(call.Fun)
	if !ok {
		return
	}
	signature, ok := w.info.Types[call.Fun].Type.(*types.Signature)
	if !ok {
		return
	}
	params := signature.Params()
	for i, arg := range call.Args {
		if params.Len() == 0 {
			return
		}
		param := params.At(min(i, params.Len()-1)).Type()
		if slice, ok := param.(*types.Slice); ok && signature.Variadic() && i >= params.Len()-1 {
			param = slice.Elem()
		}
		if declaredBy, ok := w.typeContext(param); ok && declaredBy == owner {
			w.recordBehaviour(owner, arg)
		}
	}
}

func (w wiringWalker) recordBehaviour(owner string, value ast.Expr) {
	ast.Inspect(value, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			w.recordUse(owner, n.Sel)
			return !isNamePath(n.X)
		case *ast.Ident:
			w.recordUse(owner, n)
		}
		return true
	})
}

func isNamePath(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return isNamePath(e.X)
	}
	return false
}

func (w wiringWalker) recordUse(owner string, ident *ast.Ident) {
	if used, ok := w.behaviourContext(w.info.Uses[ident]); ok && used != owner {
		edge := contextEdge{owner, used}
		if _, seen := w.edges[edge]; !seen {
			w.edges[edge] = w.position(ident)
		}
	}
}

func (w wiringWalker) behaviourContext(obj types.Object) (string, bool) {
	switch o := obj.(type) {
	case *types.Func:
		if o.Pkg() != nil {
			return w.contextOf(o.Pkg().Path())
		}
	case *types.Var:
		if hasMethods(o.Type()) {
			return w.typeContext(o.Type())
		}
	}
	return "", false
}

func hasMethods(t types.Type) bool {
	return types.NewMethodSet(t).Len() > 0 || types.NewMethodSet(types.NewPointer(t)).Len() > 0
}

func (w wiringWalker) calleeContext(fun ast.Expr) (string, bool) {
	var ident *ast.Ident
	switch f := fun.(type) {
	case *ast.Ident:
		ident = f
	case *ast.SelectorExpr:
		ident = f.Sel
	default:
		return "", false
	}
	fn, ok := w.info.Uses[ident].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return "", false
	}
	return w.contextOf(fn.Pkg().Path())
}

func (w wiringWalker) typeContext(t types.Type) (string, bool) {
	if pointer, ok := t.(*types.Pointer); ok {
		t = pointer.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", false
	}
	return w.contextOf(named.Obj().Pkg().Path())
}

func (w wiringWalker) position(node ast.Node) string {
	p := w.fset.Position(node.Pos())
	if rel, err := filepath.Rel(w.moduleDir, p.Filename); err == nil {
		return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), p.Line)
	}
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}
