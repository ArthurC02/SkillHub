package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const scopeSection = "scope"

var declarableScopes = []string{"user", "operator", "worker", "content-addressed", "scoped-upstream", "platform-wide"}

var sqlParameterPattern = regexp.MustCompile(`\$\d+|(?:^|[^@\w])@[A-Z_][A-Z0-9_]*|\bSQLC\.(?:N?ARG|SLICE)\s*\(`)

func lacksWorkspaceCondition(body string) bool {
	normalized := normalizeSQL(body)
	return sqlParameterPattern.MatchString(normalized) && !strings.Contains(normalized, "WORKSPACE_ID") &&
		len(referencedTables(body)) > 0
}

func queryScopeProblems(root string) []string {
	sections, err := parseOwnerDeclaration(filepath.Join(root, "db", queryOwnersFile))
	if err != nil {
		return []string{fmt.Sprintf("db/%s: %v", queryOwnersFile, err)}
	}
	declared, ok := sections[scopeSection]
	if !ok {
		return []string{fmt.Sprintf("db/%s: missing section %q", queryOwnersFile, scopeSection)}
	}
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}

	allowed := strings.Join(declarableScopes, ", ")
	var problems []string
	for _, name := range sortedKeys(declared) {
		query, exists := queries[name]
		switch {
		case !exists:
			problems = append(problems, fmt.Sprintf(
				"db/%s: scope.%s is not a query in db/queries", queryOwnersFile, name))
		case !slices.Contains(declarableScopes, declared[name]):
			problems = append(problems, fmt.Sprintf(
				"db/%s: scope.%s = %q is not one of %s", queryOwnersFile, name, declared[name], allowed))
		case !query.unscoped:
			problems = append(problems, fmt.Sprintf(
				"db/%s: scope.%s is declared, but db/queries/%s now names workspace_id or takes no parameter; "+
					"drop the declaration", queryOwnersFile, name, query.file))
		}
	}
	for _, name := range sortedKeys(queries) {
		if _, isDeclared := declared[name]; queries[name].unscoped && !isDeclared {
			problems = append(problems, fmt.Sprintf(
				"db/queries/%s: %s takes parameters but never names workspace_id; "+
					"add the workspace condition, or declare under scope: in db/%s which of %s it serves",
				queries[name].file, name, queryOwnersFile, allowed))
		}
	}
	return problems
}
