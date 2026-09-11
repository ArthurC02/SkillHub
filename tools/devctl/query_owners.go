package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const queryOwnersFile = "query-owners.yaml"

const genImportPath = "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"

const readAllowSection = "read_allow"

type sqlQuery struct {
	file    string
	write   bool
	mutates []string
	tables  []string
}

type callSite struct {
	boundary string
	caller   string
	path     string
}

func queryOwnerProblems(root string) []string {
	identities, problems := contextTablePackages(
		filepath.Join(root, "docs", "adr", contextMapADR), "docs/adr/"+contextMapADR)
	sections, err := parseOwnerDeclaration(filepath.Join(root, "db", queryOwnersFile))
	if err != nil {
		return append(problems, fmt.Sprintf("db/%s: %v", queryOwnersFile, err))
	}
	queries, err := loadSQLQueries(filepath.Join(root, "db", "queries"))
	if err != nil {
		return []string{fmt.Sprintf("db/queries: %v", err)}
	}

	fileOwners, queryOwners := sections["files"], sections["queries"]

	for _, section := range []string{"files", "queries"} {
		for _, key := range sortedKeys(sections[section]) {
			owner := sections[section][key]

			if section == "files" && owner == "" {
				continue
			}
			if !knownBoundaryID(identities, owner) {
				problems = append(problems, fmt.Sprintf(
					"db/%s: %s.%s = %q is not a Boundary ID in ADR-032 §1", queryOwnersFile, section, key, owner))
			}
		}
	}

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

	names := map[string]bool{}
	for name := range queries {
		names[name] = true
	}
	calls, err := queryCallSites(filepath.Join(root, "apps", "platform"), names, identities)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform: %v", err))
	}

	owner := func(name string) string {
		if context, ok := queryOwners[name]; ok {
			return context
		}
		return fileOwners[queries[name].file]
	}
	ownerBoundary := func(name string) string { return owner(name) }

	for _, side := range []struct {
		write bool

		section, other  string
		verb, otherVerb string
	}{
		{write: true, section: "allow", other: readAllowSection, verb: "write", otherVerb: "read"},
		{write: false, section: readAllowSection, other: "allow", verb: "read", otherVerb: "write"},
	} {
		tolerated := sections[side.section]

		for _, name := range sortedKeys(tolerated) {
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s is forbidden; %s must stay empty after the DDD migration",
				queryOwnersFile, side.section, name, side.section))
		}
		for _, name := range sortedKeys(queries) {
			if queries[name].write != side.write {
				continue
			}
			if owner(name) == "" {
				continue
			}
			allowed := map[string]bool{}
			for _, id := range splitList(tolerated[name]) {
				if _, ok := identities[id]; ok {
					allowed[id] = true
				}
			}
			for _, site := range calls[name] {
				if site.boundary == ownerBoundary(name) || allowed[site.boundary] {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"cross-context %s: %s is owned by %q but %q %ss it at %s",
					side.verb, name, owner(name), site.caller, side.verb, site.path))
			}
		}

	}
	problems = append(problems, tableOwnershipProblems(root, sections, queries, owner, identities)...)
	problems = append(problems, immutableTableProblems(root, sections, queries)...)
	return append(problems, rawSQLProblems(root, sections[rawSQLAllowSection])...)
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

func callsFrom(sites []callSite, context string) bool {
	for _, site := range sites {
		if site.boundary == context {
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
					"db/%s: %s.%s = %q is not a Boundary ID in ADR-032 §1", queryOwnersFile, tablesSection, table, id))
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

func queryCallSites(platform string, names map[string]bool, identities map[string]packageIdentity) (map[string][]callSite, error) {
	calls := map[string][]callSite{}
	for _, tree := range []string{"internal", "cmd"} {
		base := filepath.Join(platform, tree)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			normalizedPath := filepath.ToSlash(path)
			for _, skipped := range rawSQLSkippedDirs {
				if strings.Contains(normalizedPath, "/"+skipped+"/") {
					return nil
				}
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			importsGen := false
			for _, spec := range file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err == nil && importPath == genImportPath {
					importsGen = true
					break
				}
			}

			seen := map[string]bool{}
			interfaceQueries := map[string]bool{}
			ast.Inspect(file, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.SelectorExpr:
					if importsGen && names[node.Sel.Name] {
						seen[node.Sel.Name] = true
					}
				case *ast.InterfaceType:
					for _, method := range node.Methods.List {
						for _, name := range method.Names {
							if names[name.Name] {
								seen[name.Name] = true
								interfaceQueries[name.Name] = true
							}
						}
					}
				}
				return true
			})
			if len(seen) == 0 {
				return nil
			}
			relative, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			directory := filepath.ToSlash(filepath.Dir(relative))
			boundary, known := callerBoundary(tree, directory, identities)
			if !known {
				if tree == "cmd" {
					return fmt.Errorf("apps/platform/cmd/%s calls sqlc but has no entry in commandContexts "+
						"(tools/devctl/query_owners.go); name the context whose data it touches", directory)
				}
				return fmt.Errorf("apps/platform/internal/%s calls sqlc but has no architecture identity in ADR-032 §1", directory)
			}

			for name := range seen {
				siteBoundary, caller := boundary, boundary
				if interfaceQueries[name] {
					siteBoundary, caller = "", "a query-shaped interface"
				}
				calls[name] = append(calls[name], callSite{
					boundary: siteBoundary,
					caller:   caller,
					path:     "apps/platform/" + tree + "/" + relative,
				})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return calls, nil
}

func callerBoundary(tree, directory string, identities map[string]packageIdentity) (string, bool) {
	if tree == "cmd" {
		command, _, _ := strings.Cut(directory, "/")
		boundary, ok := commandContexts[command]
		if !ok || !knownBoundaryID(identities, boundary) {
			return "", false
		}
		return boundary, true
	}
	identity, known := resolveContextPath(directory, identities)
	return identity.ID, known
}

var rawSQLEntryPoints = map[string]bool{
	"Exec": true, "Query": true, "QueryRow": true, "Queue": true,
}

var rawSQLKeywordPattern = regexp.MustCompile(`\b(?:SELECT|INSERT|UPDATE|DELETE|SET|CREATE|ALTER|DROP|TRUNCATE|WITH)\b`)

const rawSQLAllowSection = "raw_sql_allow"

var rawSQLSkippedDirs = []string{
	"apps/platform/internal/foundation/persistence/db/gen",
	"apps/platform/internal/entrypoint/api/gen",
}

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
					"raw SQL outside sqlc: %s:%d (%s) passes %q to %s; write it as a db/queries/*.sql query so ADR-033 can see it",
					relative, site.line, site.function, site.sql, site.method))
			}
			return nil
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("apps/platform/%s: %v", dir, err))
		}
	}

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
	sort.Strings(problems)
	return problems
}

type rawSQLSite struct {
	line     int
	function string
	method   string
	sql      string
}

func rawSQLCallSites(path string) ([]rawSQLSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	packageStrings := map[string]string{}
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.CONST {
			bindValueSpecs(gen.Specs, packageStrings)
		}
	}
	var sites []rawSQLSite
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		stringsByName := make(map[string]string, len(packageStrings))
		for name, value := range packageStrings {
			stringsByName[name] = value
		}
		bindLocalStrings(fn.Body, stringsByName)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !rawSQLEntryPoints[selector.Sel.Name] {
				return true
			}

			for _, arg := range call.Args {
				text, ok := stringValue(arg, stringsByName)
				if !ok {
					continue
				}
				if rawSQLKeywordPattern.MatchString(normalizeSQL(text)) {
					sites = append(sites, rawSQLSite{
						line: fset.Position(arg.Pos()).Line, function: fn.Name.Name,
						method: selector.Sel.Name, sql: sqlPrefix(text),
					})
				}
				break
			}
			return true
		})
	}
	return sites, nil
}

func bindLocalStrings(body *ast.BlockStmt, values map[string]string) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.DeclStmt:
			if gen, ok := node.Decl.(*ast.GenDecl); ok {
				bindValueSpecs(gen.Specs, values)
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if i >= len(node.Rhs) {
					break
				}
				name, ok := lhs.(*ast.Ident)
				if value, found := stringValue(node.Rhs[i], values); ok && found {
					values[name.Name] = value
				}
			}
		}
		return true
	})
}

func bindValueSpecs(specs []ast.Spec, values map[string]string) {
	for _, spec := range specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				break
			}
			if value, ok := stringValue(valueSpec.Values[i], values); ok {
				values[name.Name] = value
			}
		}
	}
}

func stringValue(expr ast.Expr, values map[string]string) (string, bool) {
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if expr.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expr.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := values[expr.Name]
		return value, ok
	case *ast.ParenExpr:
		return stringValue(expr.X, values)
	case *ast.BinaryExpr:
		if expr.Op != token.ADD {
			return "", false
		}
		left, leftOK := stringValue(expr.X, values)
		right, rightOK := stringValue(expr.Y, values)
		return left + " " + right, leftOK || rightOK
	case *ast.CallExpr:
		selector, ok := expr.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		pkg, pkgOK := selector.X.(*ast.Ident)
		if !pkgOK || pkg.Name != "fmt" || selector.Sel.Name != "Sprintf" || len(expr.Args) == 0 {
			return "", false
		}
		return stringValue(expr.Args[0], values)
	default:
		return "", false
	}
}

func sqlPrefix(sql string) string {
	flat := strings.Join(strings.Fields(sql), " ")
	if len(flat) > 60 {
		return flat[:60] + "…"
	}
	return flat
}

const contextMapADR = "ADR-032-ddd-bounded-context-governance-for-platform.md"

type architectureKind string

const (
	architectureCore         architectureKind = "Core"
	architectureSupporting   architectureKind = "Supporting"
	architectureSharedKernel architectureKind = "Shared Kernel"
	architectureGeneric      architectureKind = "Generic"
)

type packageIdentity struct {
	Product string
	Kind    architectureKind
	ID      string
	Path    string
}

var (
	contextTableHeading = "### 1. Context 對照表"
	contextTableHeader  = []string{"產品／Bounded Context", "類型", "Boundary ID", "現行 internal path", "需求 ID 前綴"}
	boundaryIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	contextPathPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*(?:/\*)?$`)

	depguardFilePattern     = regexp.MustCompile(`(?m)^\s*-\s*"\*\*/internal/([a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*)/\*\*"\s*$`)
	depguardSelectorPattern = regexp.MustCompile(`^\*\*/internal/([a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*)/\*\*$`)
)

func contextMapProblems(root string) []string {

	const adrPath, lintPath = "docs/adr/" + contextMapADR, "apps/platform/.golangci.yml"

	declared, problems := contextTablePackages(filepath.Join(root, filepath.FromSlash(adrPath)), adrPath)
	if len(declared) == 0 {
		return append(problems, fmt.Sprintf("%s: %s has no package rows", adrPath, contextTableHeading))
	}

	lint, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lintPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("%s: %v", lintPath, err))
	}
	guarded := map[string]bool{}

	for _, match := range depguardFilePattern.FindAllStringSubmatch(stripYAMLComments(string(lint)), -1) {
		guarded[match[1]] = true
	}

	internal := filepath.Join(root, "apps", "platform", "internal")
	present, err := goPackageDirs(internal)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform/internal: %v", err))
	}
	for _, path := range sortedKeys(present) {
		if _, ok := resolveContextPath(path, declared); !ok {
			problems = append(problems, fmt.Sprintf(
				"apps/platform/internal/%s is not listed in %s §1; register it before adding the package (AGENTS.md 第 11 條)",
				path, contextMapADR))
		}
	}
	for _, id := range sortedKeys(declared) {
		identity := declared[id]
		switch {
		case !selectorExists(identity.Path, present):
			problems = append(problems, fmt.Sprintf(
				"%s §1 lists Boundary ID %q at internal/%s but no Go package directory exists there", contextMapADR, id, identity.Path))
		case architectureNeedsDepguard(identity) && !guardCovers(identity.Path, guarded):
			problems = append(problems, fmt.Sprintf(
				"%s §1 gives Boundary ID %q architecture kind %q but %s has no depguard rule covering internal/%s",
				contextMapADR, id, identity.Kind, lintPath, identity.Path))
		}
	}
	for _, path := range sortedKeys(guarded) {
		if !guardedPathDeclared(path, declared) {
			problems = append(problems, fmt.Sprintf(
				"%s guards apps/platform/internal/%s but no ADR-032 §1 Boundary ID declares that path", lintPath, path))
		}
	}
	return problems
}

func contextTablePackages(path, relative string) (map[string]packageIdentity, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", relative, err)}
	}
	declared := map[string]packageIdentity{}
	var problems []string
	inTable, sawHeader := false, false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "### ") {
			inTable = strings.HasPrefix(line, contextTableHeading)
			continue
		}
		if !inTable || !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if !sawHeader {
			if !slices.Equal(cells, contextTableHeader) {
				problems = append(problems, fmt.Sprintf("%s §1 table header must be %q", relative, strings.Join(contextTableHeader, " | ")))
				return declared, problems
			}
			sawHeader = true
			continue
		}
		if len(cells) == len(contextTableHeader) && strings.Trim(cells[0], "- ") == "" {
			continue
		}
		if len(cells) != len(contextTableHeader) {
			problems = append(problems, fmt.Sprintf("%s §1 row has %d cells; want %d", relative, len(cells), len(contextTableHeader)))
			continue
		}
		kind := cells[1]
		architecture := architectureKind(kind)
		switch architecture {
		case architectureCore, architectureSupporting, architectureSharedKernel, architectureGeneric:
		default:
			problems = append(problems, fmt.Sprintf("%s §1 has unknown architecture kind %q", relative, kind))
			continue
		}
		context := cells[0]
		if strings.Trim(context, " -—–") == "" {
			context = ""
		}
		if (architecture == architectureCore || architecture == architectureSupporting) && context == "" {
			problems = append(problems, fmt.Sprintf("%s §1 kind %q requires a context name", relative, architecture))
			continue
		}
		if (architecture == architectureSharedKernel || architecture == architectureGeneric) && context != "" {
			problems = append(problems, fmt.Sprintf("%s §1 kind %q must use — instead of a context name", relative, architecture))
			continue
		}
		id, currentPath := cells[2], cells[3]
		if !boundaryIDPattern.MatchString(id) {
			problems = append(problems, fmt.Sprintf("%s §1 has invalid Boundary ID %q", relative, id))
			continue
		}
		if !contextPathPattern.MatchString(currentPath) {
			problems = append(problems, fmt.Sprintf("%s §1 has invalid internal path %q", relative, currentPath))
			continue
		}
		if _, duplicate := declared[id]; duplicate {
			problems = append(problems, fmt.Sprintf("%s §1 declares Boundary ID %q twice", relative, id))
			continue
		}
		identity := packageIdentity{Product: context, Kind: architecture, ID: id, Path: currentPath}
		for _, previous := range declared {
			if previous.Path == identity.Path {
				problems = append(problems, fmt.Sprintf("%s §1 declares internal path %q twice (%s and %s)", relative, identity.Path, previous.ID, identity.ID))
				break
			}
			if selectorsOverlap(previous.Path, identity.Path) {
				problems = append(problems, fmt.Sprintf("%s §1 internal paths %q (%s) and %q (%s) overlap", relative, previous.Path, previous.ID, identity.Path, identity.ID))
				break
			}
		}
		declared[id] = identity
	}
	if !sawHeader {
		problems = append(problems, fmt.Sprintf("%s §1 table header must be %q", relative, strings.Join(contextTableHeader, " | ")))
	}
	return declared, problems
}

func architectureNeedsDepguard(identity packageIdentity) bool {
	if identity.Kind != architectureGeneric {
		return true
	}
	return identity.ID != "api" && !isCompositionRoot(identity.ID)
}

func knownBoundaryID(identities map[string]packageIdentity, id string) bool {
	_, ok := identities[id]
	return ok
}

func resolveContextPath(path string, identities map[string]packageIdentity) (packageIdentity, bool) {
	path = strings.Trim(filepath.ToSlash(path), "/")
	var match packageIdentity
	found, longest := false, -1
	for _, identity := range identities {
		selector := strings.TrimSuffix(identity.Path, "/*")
		if path != selector && !strings.HasPrefix(path, selector+"/") {
			continue
		}
		if strings.HasSuffix(identity.Path, "/*") && path == selector {
			continue
		}
		if len(selector) > longest {
			match, found, longest = identity, true, len(selector)
		}
	}
	return match, found
}

func goPackageDirs(internal string) (map[string]bool, error) {
	present := map[string]bool{}
	err := filepath.WalkDir(internal, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return err
		}
		relative, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		present[filepath.ToSlash(relative)] = true
		return nil
	})
	return present, err
}

func selectorExists(selector string, present map[string]bool) bool {
	for path := range present {
		if _, ok := resolveContextPath(path, map[string]packageIdentity{"": {Path: selector}}); ok {
			return true
		}
	}
	return false
}

func guardCovers(selector string, guarded map[string]bool) bool {
	return guarded[strings.TrimSuffix(selector, "/*")]
}

func guardedPathDeclared(path string, identities map[string]packageIdentity) bool {
	for _, identity := range identities {
		if strings.TrimSuffix(identity.Path, "/*") == path {
			return true
		}
	}
	return false
}

func selectorsOverlap(a, b string) bool {
	a, b = strings.TrimSuffix(a, "/*"), strings.TrimSuffix(b, "/*")
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
