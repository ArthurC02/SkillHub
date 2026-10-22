package main

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

var routeTableSources = []string{
	"apps/platform/internal/entrypoint/api/apiserver/router.go",
	"apps/platform/internal/creator/workspace/http.go",
}

const routeContractFile = "contracts/openapi/public.yaml"

var (
	routeCallPattern    = regexp.MustCompile(`mux\.Handle(?:Func)?\(([^,]+),`)
	limitedRoutePattern = regexp.MustCompile(`mux\.HandleFunc\(("[^"]+"),.*\blimited\(`)
)

var routePatternConstants = map[string]struct{ file, name string }{
	"trace.IngestPath": {"apps/platform/internal/trial/evidence/token.go", "IngestPath"},
}

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
		if documented[pattern] == nil {
			problems = append(problems, fmt.Sprintf(
				"route-table: %q is mounted but has no operation in %s; iron rule 12 wants the contract "+
					"written first, and codegen cannot see this because the Go side generates models only, "+
					"so no generated router exists to notice a route", pattern, routeContractFile))
		}
	}
	for pattern := range documented {
		if !seen[pattern] {
			problems = append(problems, fmt.Sprintf(
				"route-table: %s documents %q, which nothing mounts; a client generated from this contract "+
					"would call a 404", routeContractFile, pattern))
		}
	}
	problems = append(problems, undeclaredRateLimits(root, documented)...)
	sort.Strings(problems)
	return problems
}

func undeclaredRateLimits(root string, documented map[string]map[string]bool) []string {
	relative := routeTableSources[0]
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return []string{fmt.Sprintf("route-table: %v", err)}
	}
	var problems []string
	for _, m := range limitedRoutePattern.FindAllStringSubmatch(string(data), -1) {
		pattern, _ := strconv.Unquote(m[1])
		if statuses := documented[pattern]; statuses != nil && !statuses["429"] {
			problems = append(problems, fmt.Sprintf(
				"route-table: %q is rate limited in %s but its operation in %s declares no 429; "+
					"a generated client has no branch for the one refusal that says when to come back",
				pattern, relative, routeContractFile))
		}
	}
	return problems
}

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

func goStringConst(path, name string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return "", err
	}
	value, found := constValue(file, name)
	if !found {
		return "", fmt.Errorf("%s no longer declares the constant %s", path, name)
	}
	lit, ok := value.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("%s: %s is not a string literal constant", path, name)
	}
	return strconv.Unquote(lit.Value)
}

func constValue(file *ast.File, name string) (ast.Expr, bool) {
	for _, value := range constSpecs(file) {
		for i, ident := range value.Names {
			if ident.Name == name && i < len(value.Values) {
				return value.Values[i], true
			}
		}
	}
	return nil, false
}

var (
	contractPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	contractMethodLine = regexp.MustCompile(`^    (get|put|post|patch|delete|head|options|trace):\s*$`)
	contractTopLevel   = regexp.MustCompile(`^[A-Za-z]`)
	contractStatusLine = regexp.MustCompile(`^        "(\d{3})":`)
)

func contractOperations(path string) (map[string]map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	operations := map[string]map[string]bool{}
	inPaths := false
	current, operation := "", ""
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
			current, operation = m[1], ""
			continue
		}
		if m := contractMethodLine.FindStringSubmatch(line); m != nil && current != "" {
			operation = strings.ToUpper(m[1]) + " " + current
			operations[operation] = map[string]bool{}
			continue
		}
		if m := contractStatusLine.FindStringSubmatch(line); m != nil && operation != "" {
			operations[operation][m[1]] = true
		}
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("%s has no operations under `paths:`; this check has lost half its subject", path)
	}
	return operations, nil
}
