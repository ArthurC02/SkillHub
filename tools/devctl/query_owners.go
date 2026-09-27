package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const queryOwnersFile = "query-owners.yaml"

const readAllowSection = "read_allow"

type sqlQuery struct {
	file    string
	write   bool
	mutates []string
	tables  []string

	unscoped  bool
	decisions []string
}

func queryOwnerProblems(root string) []string {
	identities, problems := architectureIdentities(root)
	sections, err := parseOwnerDeclaration(filepath.Join(root, "db", queryOwnersFile))
	if err != nil {
		return append(problems, fmt.Sprintf("db/%s: %v", queryOwnersFile, err))
	}
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}

	fileOwners, queryOwners := sections["files"], sections["queries"]
	problems = append(problems, ownerBoundaryIDProblems(sections, identities)...)
	problems = append(problems, ownerDeclarationDriftProblems(fileOwners, queryOwners, queries)...)
	problems = append(problems, unownedQueryProblems(fileOwners, queryOwners, queries)...)

	calls, err := queryCallSites(filepath.Join(root, "apps", "platform"), queryNameSet(queries), identities)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform: %v", err))
	}

	owner := func(name string) string {
		if context, ok := queryOwners[name]; ok {
			return context
		}
		return fileOwners[queries[name].file]
	}

	for _, side := range queryAccessSides {
		tolerated := sections[side.section]
		problems = append(problems, toleratedAccessProblems(side.section, tolerated)...)
		problems = append(problems, crossContextAccessProblems(side, tolerated, queries, calls, owner, identities)...)
	}
	problems = append(problems, tableOwnershipProblems(root, sections, queries, owner, identities)...)
	problems = append(problems, immutableTableProblems(root, sections, queries)...)
	return append(problems, rawSQLProblems(root, sections[rawSQLAllowSection])...)
}

func ownerBoundaryIDProblems(sections map[string]map[string]string, identities map[string]packageIdentity) []string {
	var problems []string
	for _, section := range []string{"files", "queries"} {
		for _, key := range sortedKeys(sections[section]) {
			owner := sections[section][key]

			if section == "files" && owner == "" {
				continue
			}
			if !knownBoundaryID(identities, owner) {
				problems = append(problems, fmt.Sprintf(
					"db/%s: %s.%s = %q is not a Boundary ID in the context map", queryOwnersFile, section, key, owner))
			}
		}
	}
	return problems
}

func ownerDeclarationDriftProblems(fileOwners, queryOwners map[string]string, queries map[string]sqlQuery) []string {
	var problems []string
	declaredFiles := map[string]bool{}
	for _, query := range queries {
		declaredFiles[query.file] = true
	}
	for _, file := range sortedKeys(fileOwners) {
		if !declaredFiles[file] {
			problems = append(problems, fmt.Sprintf("db/%s: files.%s has no db/queries/%s", queryOwnersFile, file, file))
		}
	}
	for _, file := range sortedKeys(declaredFiles) {
		if _, ok := fileOwners[file]; !ok {
			problems = append(problems, fmt.Sprintf("db/%s: db/queries/%s has no default owner", queryOwnersFile, file))
		}
	}
	for _, name := range sortedKeys(queryOwners) {
		if _, ok := queries[name]; !ok {
			problems = append(problems, fmt.Sprintf("db/%s: queries.%s is not a query in db/queries", queryOwnersFile, name))
		}
	}
	return problems
}

func unownedQueryProblems(fileOwners, queryOwners map[string]string, queries map[string]sqlQuery) []string {
	var problems []string
	for _, name := range sortedKeys(queries) {
		file := queries[name].file
		if _, hasFile := fileOwners[file]; !hasFile {
			continue
		}
		if _, declaredOwner := queryOwners[name]; declaredOwner || fileOwners[file] != "" {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"db/%s: queries.%s is undeclared and db/queries/%s has no default owner; "+
				"name the context that owns its main table", queryOwnersFile, name, file))
	}
	return problems
}

func queryNameSet(queries map[string]sqlQuery) map[string]bool {
	names := map[string]bool{}
	for name := range queries {
		names[name] = true
	}
	return names
}

type queryAccessSide struct {
	write   bool
	section string
	verb    string
}

var queryAccessSides = []queryAccessSide{
	{write: true, section: "allow", verb: "write"},
	{write: false, section: readAllowSection, verb: "read"},
}

func toleratedAccessProblems(section string, tolerated map[string]string) []string {
	var problems []string
	for _, name := range sortedKeys(tolerated) {
		problems = append(problems, fmt.Sprintf(
			"db/%s: %s.%s is forbidden; %s must stay empty after the DDD migration",
			queryOwnersFile, section, name, section))
	}
	return problems
}

func crossContextAccessProblems(side queryAccessSide, tolerated map[string]string, queries map[string]sqlQuery,
	calls map[string][]callSite, owner func(string) string, identities map[string]packageIdentity) []string {
	var problems []string
	for _, name := range sortedKeys(queries) {
		if queries[name].write != side.write || owner(name) == "" {
			continue
		}
		allowed := toleratedBoundaries(tolerated[name], identities)
		for _, site := range calls[name] {
			if site.boundary == owner(name) || allowed[site.boundary] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"cross-context %s: %s is owned by %q but %q %ss it at %s",
				side.verb, name, owner(name), site.caller, side.verb, site.path))
		}
	}
	return problems
}

func toleratedBoundaries(value string, identities map[string]packageIdentity) map[string]bool {
	allowed := map[string]bool{}
	for _, id := range splitList(value) {
		if _, ok := identities[id]; ok {
			allowed[id] = true
		}
	}
	return allowed
}

func immutableTableProblems(root string, sections map[string]map[string]string, queries map[string]sqlQuery) []string {
	declared, allow := sections["immutable"], sections["immutable_allow"]
	frozen, err := frozenTables(filepath.Join(root, "db", "migrations"))
	if err != nil {
		return []string{fmt.Sprintf("db/migrations: %v", err)}
	}

	if len(declared) == 0 && len(frozen) == 0 {
		return nil
	}

	var problems []string

	for _, table := range sortedKeys(frozen) {
		if _, ok := declared[table]; !ok {
			problems = append(problems, fmt.Sprintf(
				"db/%s: db/migrations freezes %s with an unconditional enforce_immutable() trigger but immutable: does not declare it; "+
					"declare it, or remove the trigger's migration first",
				queryOwnersFile, table))
		}
	}
	for _, table := range sortedKeys(declared) {
		switch {
		case strings.TrimSpace(declared[table]) == "":
			problems = append(problems, fmt.Sprintf(
				"db/%s: immutable.%s has no reason; name the invariant it carries", queryOwnersFile, table))
		case !frozen[table]:

			problems = append(problems, fmt.Sprintf(
				"db/%s: immutable.%s has no unconditional enforce_immutable() trigger in db/migrations",
				queryOwnersFile, table))
		}
	}

	for _, name := range sortedKeys(queries) {
		exempt := map[string]bool{}
		for _, table := range splitList(allow[name]) {
			exempt[table] = true
		}
		for _, table := range queries[name].mutates {
			if _, ok := declared[table]; !ok || exempt[table] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"immutable table write: %s is append-only but %s updates or deletes it in db/queries/%s",
				table, name, queries[name].file))
		}
	}

	for _, name := range sortedKeys(allow) {
		query, ok := queries[name]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"db/%s: immutable_allow.%s is not a query in db/queries", queryOwnersFile, name))
			continue
		}
		for _, table := range splitList(allow[name]) {
			switch {
			case declared[table] == "":
				problems = append(problems, fmt.Sprintf(
					"db/%s: immutable_allow.%s = %q is not a declared immutable table",
					queryOwnersFile, name, table))
			case !slices.Contains(query.mutates, table):
				problems = append(problems, fmt.Sprintf(
					"db/%s: immutable_allow.%s = %q no longer writes it; delete the entry",
					queryOwnersFile, name, table))
			}
		}
	}
	return problems
}

var immutableTriggerPattern = regexp.MustCompile(
	`(?i)CREATE TRIGGER\s+\w+\s+BEFORE UPDATE OR DELETE ON\s+(\w+)\s+FOR EACH ROW\s+EXECUTE FUNCTION enforce_immutable\(\s*\)`)

func frozenTables(dir string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	tables := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, match := range immutableTriggerPattern.FindAllStringSubmatch(string(data), -1) {
			tables[strings.ToLower(match[1])] = true
		}
	}
	return tables, nil
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseOwnerDeclaration(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sections := map[string]map[string]string{}
	current := ""
	for number, line := range strings.Split(string(data), "\n") {
		if comment := strings.Index(line, "#"); comment >= 0 {
			line = line[:comment]
		}
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			return nil, fmt.Errorf("line %d: expected `key: value`, got %q", number+1, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !strings.HasPrefix(line, " ") {
			if value != "" {
				return nil, fmt.Errorf("line %d: section %q must have no inline value", number+1, key)
			}
			if _, duplicate := sections[key]; duplicate {
				return nil, fmt.Errorf("line %d: section %q declared twice", number+1, key)
			}
			current, sections[key] = key, map[string]string{}
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("line %d: entry %q before any section", number+1, key)
		}
		if _, duplicate := sections[current][key]; duplicate {
			return nil, fmt.Errorf("line %d: %s.%s declared twice", number+1, current, key)
		}
		sections[current][key] = value
	}
	for _, section := range []string{"files", "queries", "allow", readAllowSection, "immutable", "immutable_allow"} {
		if _, ok := sections[section]; !ok {
			return nil, fmt.Errorf("missing section %q", section)
		}
	}
	return sections, nil
}

var (
	queryNamePattern  = regexp.MustCompile(`(?m)^--\s*name:\s*(\w+)\s*:\w+`)
	sqlCommentPattern = regexp.MustCompile(`--[^\n]*`)
	sqlLiteralPattern = regexp.MustCompile(`'[^']*'`)

	sqlNonTargetPattern = regexp.MustCompile(`FOR (NO KEY )?UPDATE|FOR SHARE|DO UPDATE`)
	sqlWritePattern     = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE)\b`)
	sqlMutatePattern    = regexp.MustCompile(`\b(?:UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?([A-Z_][A-Z0-9_]*)`)
	sqlTableRefPattern  = regexp.MustCompile(`\b(?:FROM|JOIN|INTO|UPDATE|USING)\s+(?:ONLY\s+)?([A-Z_][A-Z0-9_]*)`)
	sqlTableListPattern = regexp.MustCompile(
		`\b(?:FROM|USING)\s+(?:ONLY\s+)?[A-Z_][A-Z0-9_]*(?:\s+(?:AS\s+)?[A-Z_][A-Z0-9_]*)?((?:\s*,\s*[A-Z_][A-Z0-9_]*(?:\s+(?:AS\s+)?[A-Z_][A-Z0-9_]*)?)+)`)
	sqlListedTablePattern = regexp.MustCompile(`,\s*([A-Z_][A-Z0-9_]*)`)
	sqlCTEPattern         = regexp.MustCompile(`(?:\bWITH\s+(?:RECURSIVE\s+)?|,\s*)([A-Z_][A-Z0-9_]*)\s+AS\s*(?:NOT\s+)?(?:MATERIALIZED\s*)?\(`)

	createTablePattern    = regexp.MustCompile(`(?i)CREATE TABLE\s+(?:IF NOT EXISTS\s+)?(\w+)`)
	partitionTablePattern = regexp.MustCompile(`(?i)CREATE TABLE\s+(?:IF NOT EXISTS\s+)?(\w+)\s+PARTITION OF\b`)
)

func loadSQLQueries(dir string) (map[string]sqlQuery, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	queries := map[string]sqlQuery{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		text := string(data)
		matches := queryNamePattern.FindAllStringSubmatchIndex(text, -1)
		for i, match := range matches {
			end := len(text)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			name := text[match[2]:match[3]]
			if _, duplicate := queries[name]; duplicate {
				return nil, fmt.Errorf("query %s is declared in two files", name)
			}
			body := text[match[1]:end]
			queries[name] = sqlQuery{
				file:    filepath.Base(path),
				write:   isWriteStatement(body),
				mutates: mutatedTables(body),
				tables:  referencedTables(body),

				unscoped:  lacksWorkspaceCondition(body),
				decisions: sqlDecisionsOf(body),
			}
		}
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries found")
	}
	return queries, nil
}

func isWriteStatement(body string) bool {
	return sqlWritePattern.MatchString(normalizeSQL(body))
}

// normalizeSQL strips comments, string literals and row-lock/upsert clauses
// before verb matching, so none of them can be mistaken for a write keyword.
func normalizeSQL(body string) string {
	body = sqlCommentPattern.ReplaceAllString(body, " ")
	body = sqlLiteralPattern.ReplaceAllString(body, " ")
	return sqlNonTargetPattern.ReplaceAllString(strings.ToUpper(body), " ")
}

func mutatedTables(body string) []string {
	var tables []string
	for _, match := range sqlMutatePattern.FindAllStringSubmatch(normalizeSQL(body), -1) {
		tables = append(tables, strings.ToLower(match[1]))
	}
	return tables
}

const tablesSection = "tables"

func referencedTables(body string) []string {
	sql := normalizeSQL(body)
	ctes := map[string]bool{}
	for _, match := range sqlCTEPattern.FindAllStringSubmatch(sql, -1) {
		ctes[match[1]] = true
	}
	seen := map[string]bool{}
	var tables []string
	add := func(name string) {
		if !ctes[name] && !seen[name] {
			seen[name] = true
			tables = append(tables, strings.ToLower(name))
		}
	}
	for _, match := range sqlTableRefPattern.FindAllStringSubmatch(sql, -1) {
		add(match[1])
	}
	for _, list := range sqlTableListPattern.FindAllStringSubmatch(sql, -1) {
		for _, match := range sqlListedTablePattern.FindAllStringSubmatch(list[1], -1) {
			add(match[1])
		}
	}
	return tables
}

func createdTables(dir string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	tables := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		text := sqlCommentPattern.ReplaceAllString(string(data), " ")
		for _, match := range createTablePattern.FindAllStringSubmatch(text, -1) {
			tables[strings.ToLower(match[1])] = true
		}
		for _, match := range partitionTablePattern.FindAllStringSubmatch(text, -1) {
			delete(tables, strings.ToLower(match[1]))
		}
	}
	return tables, nil
}

func tableOwnershipProblems(root string, sections map[string]map[string]string, queries map[string]sqlQuery,
	owner func(string) string, identities map[string]packageIdentity) []string {
	created, err := createdTables(filepath.Join(root, "db", "migrations"))
	if err != nil {
		return []string{fmt.Sprintf("db/migrations: %v", err)}
	}
	declared, ok := sections[tablesSection]
	if !ok {
		if len(created) == 0 {
			return nil
		}
		return []string{fmt.Sprintf(
			"db/%s: missing section %q; every table needs the context that owns it", queryOwnersFile, tablesSection)}
	}

	var problems []string
	for _, table := range sortedKeys(created) {
		if _, ok := declared[table]; !ok {
			problems = append(problems, fmt.Sprintf(
				"db/%s: db/migrations creates %s but %s: does not name the context that owns it",
				queryOwnersFile, table, tablesSection))
		}
	}
	owners := map[string][]string{}
	for _, table := range sortedKeys(declared) {
		if !created[table] {
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s is not created by any migration", queryOwnersFile, tablesSection, table))
		}
		ids := splitList(declared[table])
		if len(ids) == 0 {
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s names no owner", queryOwnersFile, tablesSection, table))
		}
		for _, id := range ids {
			if !knownBoundaryID(identities, id) {
				problems = append(problems, fmt.Sprintf(
					"db/%s: %s.%s = %q is not a Boundary ID in the context map", queryOwnersFile, tablesSection, table, id))
			}
		}
		owners[table] = ids
	}

	for _, name := range sortedKeys(queries) {
		context := owner(name)
		if context == "" {
			continue
		}
		for _, table := range queries[name].tables {
			ids, ok := owners[table]
			if !ok || slices.Contains(ids, context) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"cross-context table: %s is owned by %q but its SQL touches %s, which belongs to %s (db/queries/%s)",
				name, context, table, strings.Join(ids, ", "), queries[name].file))
		}
	}
	return problems
}

var commandContexts = map[string]string{

	"reindex": "catalog",
}

const rawSQLAllowSection = "raw_sql_allow"

func rawSQLProblems(root string, allow map[string]string) []string {
	var problems []string
	hit := map[string]bool{}

	for _, dir := range []string{"internal", "cmd"} {
		base := filepath.Join(root, "apps", "platform", dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			for _, skipped := range rawSQLSkippedDirs {
				if strings.HasPrefix(relative, skipped+"/") {
					return nil
				}
			}
			found, err := rawSQLCallSites(path)
			if err != nil {
				return err
			}
			for _, site := range found {
				key := relative + "@" + site.function
				if _, exempt := allow[key]; exempt {
					hit[key] = true
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"raw SQL outside sqlc: %s:%d (%s) passes %q to %s; write it as a db/queries/*.sql query so the query-owner check can see it",
					relative, site.line, site.function, site.sql, site.method))
			}
			return nil
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("apps/platform/%s: %v", dir, err))
		}
	}

	problems = append(problems, rawSQLAllowEntryProblems(allow, hit)...)
	sort.Strings(problems)
	return problems
}

func rawSQLAllowEntryProblems(allow map[string]string, hit map[string]bool) []string {
	var problems []string
	for _, key := range sortedKeys(allow) {
		switch {
		case strings.TrimSpace(allow[key]) == "":
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s has no reason; name why it cannot be a sqlc query",
				queryOwnersFile, rawSQLAllowSection, key))
		case !hit[key]:
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s no longer contains raw SQL; delete the entry",
				queryOwnersFile, rawSQLAllowSection, key))
		}
	}
	return problems
}
