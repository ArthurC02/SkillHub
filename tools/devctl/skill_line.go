package main

import (
	"fmt"
	"path/filepath"
	"regexp"
)

var (
	readsSkillVersions = regexp.MustCompile(`\b(?:FROM|JOIN)\s+SKILL_VERSIONS\b`)
	readsSkillLine     = regexp.MustCompile(`\bSKILL_LINE_VERSIONS\b`)
	versionOrdering    = regexp.MustCompile(
		`\bORDER\s+BY\b[^;]*\bVERSION_NUMBER\b` +
			`|\bMAX\s*\(\s*(?:[A-Z_][A-Z0-9_]*\.)?VERSION_NUMBER\b` +
			`|\bVERSION_NUMBER\s*[<>]`)
)

var versionOrderingOverEveryVersion = map[string]string{
	"ListSkillVersions":           "the version list shows held candidates too",
	"ListSourcePaths":             "every stored version's source path, held or not",
	"SkillsFromSameStoredPackage": "which Skills share a stored package, whatever their line",
	"GetHighestVersionNumber":     "the next version number stays unique across held and line versions",
}

func ordersVersionsOutsideLine(body string) bool {
	normalized := normalizeSQL(body)
	return readsSkillVersions.MatchString(normalized) && versionOrdering.MatchString(normalized) &&
		!readsSkillLine.MatchString(normalized)
}

func skillLineProblems(root string) []string {
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}
	return lineBypasses(queries, versionOrderingOverEveryVersion)
}

func lineBypasses(queries map[string]sqlQuery, overEveryVersion map[string]string) []string {
	var problems []string
	for _, name := range sortedKeys(queries) {
		_, allowed := overEveryVersion[name]
		if queries[name].ordersVersionsOutsideLine && !allowed {
			problems = append(problems, fmt.Sprintf(
				"db/queries/%s: %s orders skill_versions by version_number without reading skill_line_versions, "+
					"so a held candidate could become its Skill's newest, latest or previous version; read the view, "+
					"or name the query in versionOrderingOverEveryVersion with why every version counts",
				queries[name].file, name))
		}
	}
	for _, name := range sortedKeys(overEveryVersion) {
		if query, exists := queries[name]; !exists || !query.ordersVersionsOutsideLine {
			problems = append(problems, fmt.Sprintf(
				"tools/devctl: versionOrderingOverEveryVersion names %s, which no longer orders skill_versions "+
					"outside the line; drop the entry", name))
		}
	}
	return problems
}
