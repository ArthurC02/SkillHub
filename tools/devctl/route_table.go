package main

// The mounted route table and contracts/openapi/public.yaml, reconciled both ways.
//
// Iron rule 12 says the contract comes first and CI checks the drift by
// codegen. For public.yaml that is only half true: `devctl gen --check`
// regenerates types (ogen models, the TS client, the Python stubs), and types
// say nothing about which routes exist. Exactly one operation reaches the
// generated server (`GET /healthz`, see router.go); the other 71 are mounted by
// hand. So a route added to NewRouter without a `paths:` entry, or a `paths:`
// entry for a route nobody mounts, is drift that regeneration cannot see and
// redocly cannot see either — redocly validates the schema against itself.
//
// apiserver's own TestEveryMountedRouteIsInTheAnonymousMatrix already reads the
// same two files, for the anonymous-status matrix. It cannot do this job: it
// lives in a package whose 267 other tests need a database, so on a laptop
// without one the whole file is skipped. This runs in `automation-check`, which
// runs on every push and needs nothing.
//
// The scan is source text, not a built mux, for the reason that test records:
// http.ServeMux does not expose its patterns, and a route DELETED from the
// source is one of the two things being looked for.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The two files that mount routes: NewRouter's own table, and the auth routes it
// delegates to workspace.Handler.Mount.
var routeTableSources = []string{
	"apps/platform/internal/entrypoint/api/apiserver/router.go",
	"apps/platform/internal/creator/workspace/http.go",
}

const routeContractFile = "contracts/openapi/public.yaml"

// routeCallPattern captures the pattern argument of every mux.Handle /
// mux.HandleFunc call: everything up to the first comma, because one pattern is
// a concatenation rather than a single literal.
var routeCallPattern = regexp.MustCompile(`mux\.Handle(?:Func)?\(([^,]+),`)

// THE ONE DOCUMENTED EXCEPTION.
//
// Every route pattern in the table is a string literal except one:
//
//	mux.HandleFunc("POST "+trace.IngestPath+"{token}", d.Trace.Ingest)
//
// trace.IngestPath is a constant in apps/platform/internal/trial/evidence
// because the token minter builds the callback URL from the same value, and two
// copies of a URL prefix that must match is the drift this file is about.
//
// The exception is resolved rather than skipped, and its source is the constant
// rather than a copy of the constant here: a scan that quietly drops a route is
// a scan that enforces nothing, and a hard-coded "/internal/trace/" would go
// stale the same way. An identifier this table does not know is a LOUD failure —
// never a skipped route.
var routePatternConstants = map[string]struct{ file, name string }{
	"trace.IngestPath": {"apps/platform/internal/trial/evidence/token.go", "IngestPath"},
}

// The floor from apiserver's own scan sanity check, for the same reason: a
// broken regex passes a both-ways comparison by finding nothing on both sides.
const routeTableFloor = 60

func routeTableProblems(root string) []string {
	mounted, problems := mountedRoutes(root)
	if len(problems) > 0 {
		return problems
	}
	if len(mounted) < routeTableFloor {
		return []string{fmt.Sprintf(
			"route-table: the scan found %d mounted routes in %s; the table has had more than %d since M4, "+
				"so the scan is broken rather than the table shrunk",
			len(mounted), strings.Join(routeTableSources, " + "), routeTableFloor)}
	}

	documented, err := contractOperations(filepath.Join(root, filepath.FromSlash(routeContractFile)))
	if err != nil {
		return []string{fmt.Sprintf("route-table: %v", err)}
	}
	if len(documented) < routeTableFloor {
		return []string{fmt.Sprintf(
			"route-table: %s declares %d operations under `paths:`; the same floor applies to this side, "+
				"so the YAML scan is broken rather than the contract shrunk", routeContractFile, len(documented))}
	}

	seen := map[string]bool{}
	for _, pattern := range mounted {
		if seen[pattern] {
			problems = append(problems, fmt.Sprintf(
				"route-table: %q is mounted twice; http.ServeMux panics on that at startup", pattern))
			continue
		}
		seen[pattern] = true
		if !documented[pattern] {
			problems = append(problems, fmt.Sprintf(
				"route-table: %q is mounted but has no operation in %s; iron rule 12 wants the contract "+
					"written first, and codegen cannot see this because only GET /healthz reaches the "+
					"generated server", pattern, routeContractFile))
		}
	}
	for pattern := range documented {
		if !seen[pattern] {
			problems = append(problems, fmt.Sprintf(
				"route-table: %s documents %q, which nothing mounts; a client generated from this contract "+
					"would call a 404", routeContractFile, pattern))
		}
	}
	sort.Strings(problems)
	return problems
}

// mountedRoutes returns every "METHOD /path" the two route tables mount.
func mountedRoutes(root string) ([]string, []string) {
	var out []string
	var problems []string
	for _, relative := range routeTableSources {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, []string{fmt.Sprintf("route-table: %v", err)}
		}
		for _, m := range routeCallPattern.FindAllStringSubmatch(string(data), -1) {
			pattern, err := resolveRoutePattern(root, m[1])
			if err != nil {
				problems = append(problems, fmt.Sprintf("route-table: %s: %v", relative, err))
				continue
			}
			method, path, ok := strings.Cut(pattern, " ")
			if !ok || !strings.HasPrefix(path, "/") || strings.ToUpper(method) != method {
				problems = append(problems, fmt.Sprintf(
					"route-table: %s mounts %q, which is not a `METHOD /path` pattern; the scan is "+
						"matching something else", relative, pattern))
				continue
			}
			out = append(out, pattern)
		}
	}
	return out, problems
}

func resolveRoutePattern(root, expr string) (string, error) {
	var b strings.Builder
	for _, part := range strings.Split(expr, "+") {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`) {
			unquoted, err := strconv.Unquote(part)
			if err != nil {
				return "", fmt.Errorf("route pattern fragment %s is not a readable string: %w", part, err)
			}
			b.WriteString(unquoted)
			continue
		}
		known, ok := routePatternConstants[part]
		if !ok {
			return "", fmt.Errorf(
				"route pattern %s is built from %q, which is neither a literal nor a known constant; "+
					"add it to routePatternConstants in tools/devctl/route_table.go or this scan is "+
					"silently missing a route", expr, part)
		}
		value, err := goStringConst(filepath.Join(root, filepath.FromSlash(known.file)), known.name)
		if err != nil {
			return "", err
		}
		b.WriteString(value)
	}
	return b.String(), nil
}

// goStringConst reads one string constant's value out of the AST. Same reason
// require_db_guard.go gives for parsing rather than grepping: a text search
// finds the constant's own doc comment and survives the constant's deletion.
func goStringConst(path, name string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name || i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return "", fmt.Errorf("%s: %s is not a string literal constant", path, name)
				}
				return strconv.Unquote(lit.Value)
			}
		}
	}
	return "", fmt.Errorf("%s no longer declares the constant %s", path, name)
}

var (
	contractPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	contractMethodLine = regexp.MustCompile(`^    (get|put|post|patch|delete|head|options|trace):\s*$`)
	contractTopLevel   = regexp.MustCompile(`^[A-Za-z]`)
)

// contractOperations reads the `paths:` block of an OpenAPI document by shape
// rather than with a YAML parser, which is the same trade isolation_levels.go
// makes: devctl builds anywhere precisely because it has no dependencies. Two
// levels of indentation are all this needs, and the floor above is what notices
// when the shape stops matching.
func contractOperations(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	operations := map[string]bool{}
	inPaths := false
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if inPaths && contractTopLevel.MatchString(line) {
			break
		}
		if !inPaths {
			continue
		}
		if m := contractPathLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := contractMethodLine.FindStringSubmatch(line); m != nil && current != "" {
			operations[strings.ToUpper(m[1])+" "+current] = true
		}
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("%s has no operations under `paths:`; this check has lost half its subject", path)
	}
	return operations, nil
}
