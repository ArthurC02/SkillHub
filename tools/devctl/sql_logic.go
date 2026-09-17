package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	sqlWhenPattern          = regexp.MustCompile(`\bWHEN\b`)
	sqlParameterWhenPattern = regexp.MustCompile(`\bWHEN\s+(?:SQLC\.N?ARG\(\s*\w+\s*\)|@\w+|\$\d+)(?:::\w+)?\s+THEN\b`)
	sqlLiteralListPattern   = regexp.MustCompile(`\bIN\s*\(\s*'`)
	sqlIntervalPattern      = regexp.MustCompile(`\bINTERVAL\s*'|'::INTERVAL\b`)
)

var sqlDecisionConstructs = []struct {
	name    string
	present func(sql string) bool
}{
	{"a CASE branching on data", func(sql string) bool {
		return len(sqlWhenPattern.FindAllStringIndex(sql, -1)) > len(sqlParameterWhenPattern.FindAllStringIndex(sql, -1))
	}},
	{"a literal IN list", sqlLiteralListPattern.MatchString},
	{"an interval literal", sqlIntervalPattern.MatchString},
}

func sqlDecisionsOf(body string) []string {
	sql := strings.ToUpper(sqlCommentPattern.ReplaceAllString(body, " "))
	var found []string
	for _, construct := range sqlDecisionConstructs {
		if construct.present(sql) {
			found = append(found, construct.name)
		}
	}
	return found
}

func sqlLogicProblems(root string) []string {
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}
	return sqlDecisionProblems(queries)
}

func sqlDecisionProblems(queries map[string]sqlQuery) []string {
	var problems []string
	for _, name := range sortedKeys(queries) {
		if decisions := queries[name].decisions; len(decisions) > 0 {
			problems = append(problems, fmt.Sprintf(
				"db/queries/%s: %s decides in SQL with %s; decide in Go and pass the result as a parameter",
				queries[name].file, name, strings.Join(decisions, ", ")))
		}
	}
	return problems
}
