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
	problems = append(problems, immutableTableProblems(root, sections, queries)...)
	return append(problems, rawSQLProblems(root, sections[rawSQLAllowSection])...)
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

// ── 裸 SQL tripwire ────────────────────────────────────────────────────────
//
// 上面兩道檢查（ownership 與 immutable）都只看 db/queries/*.sql，前提是
// **所有寫入都經過 sqlc**。這個前提目前成立，但沒有任何東西強制它：一行
// `tx.Exec(ctx, "UPDATE skills SET ...")` 同時繞過 owner 宣告與 immutable 宣告，
// 而且不會有人發現。這道檢查就是補那個洞。
//
// 抓得到：字面值直接交給 pgx 入口的 DML。
// **抓不到**（這是 tripwire，不是證明，不要當它完備）：
//   - `const q = "UPDATE ..."` 之後 `tx.Exec(ctx, q)`——SQL 不在呼叫點；
//     apps/platform/internal/run/halt.go 的 reconcilerLastRun 就是這個形狀
//     （它是 SELECT，不是違規，但同一個形狀換成 DML 這裡看不到）。
//   - `fmt.Sprintf` / 字串串接組出來的 SQL（只看得到拼進去的字面片段）。
//   - 走 database/sql、River migration、或 psql 之類 pgx 以外的路徑。
//   - apps/platform 以外的程式。
// 要真正封死只能走型別（把 Pool 收在只暴露 sqlc 的 wrapper 後面）；那是另一個
// 決定，不是這道檢查。
//
// read 本來就不強制（ADR-033），所以裸 SELECT 一律放行；
// eval/reconcile.go 的裸 SELECT 是已知的 read 盲點，記在 ADR-033 註記裡。

// pgx 會收 SQL 字面值的入口。SendBatch 收的是 *pgx.Batch 不是 SQL，
// 真正帶 SQL 的是 Batch.Queue；CopyFrom 收的是識別字不是語句，故不列入。
var rawSQLEntryPoints = map[string]bool{
	"Exec": true, "Query": true, "QueryRow": true, "Queue": true,
}

// 具名豁免所在的 section：key 是 repo 相對路徑，value 是理由（不得留空）。
// 形狀比照 allow:／immutable_allow:——具名、有理由、失效即 FAIL。
// section 不存在＝零條豁免，是最嚴格的預設，所以不列入必要 section。
const rawSQLAllowSection = "raw_sql_allow"

// 生成碼自己就是 sqlc／ogen 的輸出，不受這道檢查約束。
var rawSQLSkippedDirs = []string{
	"apps/platform/internal/platform/db/gen",
	"apps/platform/internal/api/gen",
}

func rawSQLProblems(root string, allow map[string]string) []string {
	var problems []string
	hit := map[string]bool{}

	for _, dir := range []string{"internal", "cmd"} {
		base := filepath.Join(root, "apps", "platform", dir)
		if _, err := os.Stat(base); err != nil {
			continue // 測試 fixture 不一定兩個都有；真的缺了 ownership 檢查會先喊。
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
			found, err := rawDMLCallSites(path)
			if err != nil {
				return err
			}
			for _, site := range found {
				if _, exempt := allow[relative]; exempt {
					hit[relative] = true
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"raw DML outside sqlc: %s:%d passes %q to %s; write it as a db/queries/*.sql query so ADR-033 can see it",
					relative, site.line, site.sql, site.method))
			}
			return nil
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("apps/platform/%s: %v", dir, err))
		}
	}

	// 失效的豁免要清掉，否則它默默罩住下一個寫進同一個檔的裸 DML。
	for _, relative := range sortedKeys(allow) {
		switch {
		case strings.TrimSpace(allow[relative]) == "":
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s has no reason; name why it cannot be a sqlc query",
				queryOwnersFile, rawSQLAllowSection, relative))
		case !hit[relative]:
			problems = append(problems, fmt.Sprintf(
				"db/%s: %s.%s no longer contains raw DML; delete the entry",
				queryOwnersFile, rawSQLAllowSection, relative))
		}
	}
	sort.Strings(problems)
	return problems
}

type rawSQLSite struct {
	line   int
	method string
	sql    string
}

// rawDMLCallSites 解析單一 Go 檔，回報把含 DML 的字面值交給 pgx 入口的呼叫。
// 判定沿用 isWriteStatement——SQL 只有一套解析。
func rawDMLCallSites(path string) ([]rawSQLSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var sites []rawSQLSite
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !rawSQLEntryPoints[selector.Sel.Name] {
			return true
		}
		// SQL 是第一個帶字面值的引數（Exec(ctx, sql, …) 或 Queue(sql, …)）；
		// 後面的是參數值，把它們一起看會讓 "delete" 這種值變成假警報。
		for _, arg := range call.Args {
			text := stringLiterals(arg)
			if text == "" {
				continue
			}
			if isWriteStatement(text) {
				sites = append(sites, rawSQLSite{
					line:   fset.Position(arg.Pos()).Line,
					method: selector.Sel.Name,
					sql:    sqlPrefix(text),
				})
			}
			break
		}
		return true
	})
	return sites, nil
}

// stringLiterals 把一個引數子樹裡的字串字面值串起來，所以 `"UPDATE " + table`
// 的常數半邊仍然看得到。非字面值的部分就是看不到——見上面的界限說明。
func stringLiterals(node ast.Node) string {
	var parts []string
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if value, err := strconv.Unquote(lit.Value); err == nil {
				parts = append(parts, value)
			}
		}
		return true
	})
	return strings.Join(parts, " ")
}

// sqlPrefix 把多行 SQL 壓成一行開頭，讓失敗訊息認得出是哪一段。
func sqlPrefix(sql string) string {
	flat := strings.Join(strings.Fields(sql), " ")
	if len(flat) > 60 {
		return flat[:60] + "…"
	}
	return flat
}
