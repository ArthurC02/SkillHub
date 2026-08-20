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

// sqlc generates every query in db/queries/*.sql into one Go package, so the
// depguard rules in apps/platform/.golangci.yml — which only see imports —
// cannot tell `run` calling its own query from `run` writing eval's table.
// db/query-owners.yaml names the owning context of each query and this check is
// what makes that declaration binding. See ADR-033.
const queryOwnersFile = "query-owners.yaml"

// The Bounded Contexts of ADR-032 §1 plus the generic packages listed there.
// A context name outside this set is almost always a typo in the declaration,
// and a silently ignored typo would open exactly the hole this check closes.
var platformContexts = map[string]bool{
	"identity": true, "catalog": true, "registry": true, "skillpkg": true,
	"ingest": true, "testlab": true, "run": true, "eval": true,
	"packaging": true, "trace": true, "policy": true, "analytics": true,
	"audit": true, "outbox": true, "objreconcile": true, "llmclient": true,
	"apiserver": true, "api": true, "platform": true,
}

// Only files that import the sqlc package can call a query, which is also what
// keeps the generated packages themselves out of the scan: db/gen does not
// import itself and api/gen never imports it.
const genImportPath = "internal/platform/db/gen\""

type sqlQuery struct {
	file    string   // basename of the db/queries/*.sql file it lives in
	write   bool     // INSERT / UPDATE / DELETE, including CTE writes
	mutates []string // tables it UPDATEs or DELETEs FROM, lower-cased
}

type callSite struct {
	context string
	path    string
}

func queryOwnerProblems(root string) []string {
	sections, err := parseOwnerDeclaration(filepath.Join(root, "db", queryOwnersFile))
	if err != nil {
		return []string{fmt.Sprintf("db/%s: %v", queryOwnersFile, err)}
	}
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}

	var problems []string
	fileOwners, queryOwners, allow := sections["files"], sections["queries"], sections["allow"]

	for _, section := range []string{"files", "queries"} {
		for _, key := range sortedKeys(sections[section]) {
			if owner := sections[section][key]; !platformContexts[owner] {
				problems = append(problems, fmt.Sprintf(
					"db/%s: %s.%s = %q is not a context in ADR-032 §1", queryOwnersFile, section, key, owner))
			}
		}
	}

	// A declaration that drifts from db/queries stops being a defence, so both
	// directions are errors: an undeclared query and a declaration of a query
	// that no longer exists.
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
	for _, name := range sortedKeys(allow) {
		if _, ok := queries[name]; !ok {
			problems = append(problems, fmt.Sprintf("db/%s: allow.%s is not a query in db/queries", queryOwnersFile, name))
		}
	}

	names := map[string]bool{}
	for name := range queries {
		names[name] = true
	}
	calls, err := queryCallSites(filepath.Join(root, "apps", "platform", "internal"), names)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform/internal: %v", err))
	}

	owner := func(name string) string {
		if context, ok := queryOwners[name]; ok {
			return context
		}
		return fileOwners[queries[name].file]
	}

	// Reads stay unenforced on purpose (ADR-033): a write is where a foreign
	// context can break an invariant it does not know about.
	for _, name := range sortedKeys(queries) {
		if !queries[name].write {
			continue
		}
		allowed := map[string]bool{}
		for _, context := range splitList(allow[name]) {
			allowed[context] = true
		}
		for _, site := range calls[name] {
			if site.context == owner(name) || allowed[site.context] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"cross-context write: %s is owned by %q but %q writes it at %s",
				name, owner(name), site.context, site.path))
		}
	}

	// A tolerated drift whose call site is gone must be deleted from the file;
	// leaving it there quietly re-opens the hole for the next caller.
	for _, name := range sortedKeys(allow) {
		query, ok := queries[name]
		if !ok {
			continue // already reported above
		}
		for _, context := range splitList(allow[name]) {
			switch {
			case !platformContexts[context]:
				problems = append(problems, fmt.Sprintf(
					"db/%s: allow.%s = %q is not a context in ADR-032 §1", queryOwnersFile, name, context))
			case !query.write:
				problems = append(problems, fmt.Sprintf(
					"db/%s: allow.%s is a read query; reads are not enforced", queryOwnersFile, name))
			case context == owner(name):
				problems = append(problems, fmt.Sprintf(
					"db/%s: allow.%s = %q is the owner; the entry is redundant", queryOwnersFile, name, context))
			case !callsFrom(calls[name], context):
				problems = append(problems, fmt.Sprintf(
					"db/%s: allow.%s = %q no longer calls it; delete the entry", queryOwnersFile, name, context))
			}
		}
	}
	return append(problems, immutableTableProblems(root, sections, queries)...)
}

// Postgres already refuses these writes: db/migrations attaches
// enforce_immutable() to every frozen table (0005, 0013, 0027, 0033) and
// db/tests/immutability_test.sql proves it. This check exists because that
// refusal arrives at runtime, in whatever environment ran the statement first —
// a `UPDATE skill_versions SET ...` merged today surfaces as a 500 in staging,
// not as a red build on the pull request that wrote it.
//
// The declaration lives in db/query-owners.yaml rather than being derived from
// the triggers so that dropping a trigger cannot quietly retire the rule: the
// two sides are cross-checked below, and weakening either alone fails CI.
func immutableTableProblems(root string, sections map[string]map[string]string, queries map[string]sqlQuery) []string {
	declared, allow := sections["immutable"], sections["immutable_allow"]
	if len(declared) == 0 {
		return nil
	}
	frozen, err := frozenTables(filepath.Join(root, "db", "migrations"))
	if err != nil {
		return []string{fmt.Sprintf("db/migrations: %v", err)}
	}

	var problems []string
	for _, table := range sortedKeys(declared) {
		switch {
		case strings.TrimSpace(declared[table]) == "":
			problems = append(problems, fmt.Sprintf(
				"db/%s: immutable.%s has no reason; name the invariant it carries", queryOwnersFile, table))
		case !frozen[table]:
			// Either a typo, or somebody dropped the trigger. Both mean this
			// entry is promising an enforcement that no longer exists.
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

	// Same reasoning as the allow section above: an exemption whose statement
	// is gone must go with it, or it silently covers the next one written.
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

// A trigger with a WHEN clause or mutable-column arguments freezes part of a
// row, not the table (`runs` after it goes terminal, `evaluations` after it
// completes). Only the argument-less, condition-less form means "insert-only",
// which is the shape this check can reason about from SQL text alone.
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

func callsFrom(sites []callSite, context string) bool {
	for _, site := range sites {
		if site.context == context {
			return true
		}
	}
	return false
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

// parseOwnerDeclaration reads the two-level `section:` / `  key: value` subset
// of YAML that db/query-owners.yaml is written in. devctl has no dependencies
// at all and this shape does not justify the first one (ADR-033); anything
// outside the subset is rejected rather than silently skipped.
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
	for _, section := range []string{"files", "queries", "allow", "immutable", "immutable_allow"} {
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
	// UPDATE that names no table: a row lock, or the upsert branch of an
	// INSERT. Removing them keeps `FOR UPDATE` out of isWriteStatement and
	// `DO UPDATE SET` out of mutatedTables, which would otherwise read `SET`
	// as the table being written.
	sqlNonTargetPattern = regexp.MustCompile(`FOR (NO KEY )?UPDATE|FOR SHARE|DO UPDATE`)
	sqlWritePattern     = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE)\b`)
	sqlMutatePattern    = regexp.MustCompile(`\b(?:UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?([A-Z_][A-Z0-9_]*)`)
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
			}
		}
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries found")
	}
	return queries, nil
}

// isWriteStatement looks for a write verb anywhere in the body rather than at
// the start, because a `WITH ... DELETE FROM` body (governance.sql's purge)
// opens on a SELECT. Comments, string literals and row locks come out first:
// all three can carry the verb without the statement writing anything.
func isWriteStatement(body string) bool {
	return sqlWritePattern.MatchString(normalizeSQL(body))
}

// normalizeSQL strips everything that can carry a write verb without being one:
// comments, string literals, row locks and upsert branches. Upper-cased so the
// patterns above need no case folding of their own.
func normalizeSQL(body string) string {
	body = sqlCommentPattern.ReplaceAllString(body, " ")
	body = sqlLiteralPattern.ReplaceAllString(body, " ")
	return sqlNonTargetPattern.ReplaceAllString(strings.ToUpper(body), " ")
}

// mutatedTables names the tables a query UPDATEs or DELETEs FROM. INSERT is not
// a mutation here: an append-only table is exactly one that accepts inserts and
// nothing else.
func mutatedTables(body string) []string {
	var tables []string
	for _, match := range sqlMutatePattern.FindAllStringSubmatch(normalizeSQL(body), -1) {
		tables = append(tables, strings.ToLower(match[1]))
	}
	return tables
}

func queryCallSites(internal string, names map[string]bool) (map[string][]callSite, error) {
	alternatives := make([]string, 0, len(names))
	for name := range names {
		alternatives = append(alternatives, regexp.QuoteMeta(name))
	}
	sort.Strings(alternatives)
	callPattern := regexp.MustCompile(`\.(` + strings.Join(alternatives, "|") + `)\(`)

	calls := map[string][]callSite{}
	err := filepath.WalkDir(internal, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, genImportPath) {
			return nil
		}
		relative, err := filepath.Rel(internal, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		context, _, _ := strings.Cut(relative, "/")
		// One entry per query per file: the report names the file, and a second
		// call on the next line adds nothing to it.
		seen := map[string]bool{}
		for _, match := range callPattern.FindAllStringSubmatch(text, -1) {
			if seen[match[1]] {
				continue
			}
			seen[match[1]] = true
			calls[match[1]] = append(calls[match[1]], callSite{context: context, path: "apps/platform/internal/" + relative})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return calls, nil
}
