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

// 跨 context read 的具名豁免所在 section（ADR-035）。形狀與 `allow:` 完全一致：
// key 是 query 名、value 是被容忍的呼叫端 context 清單，理由寫在上方的分組註解。
// section 不存在＝零條豁免，是最嚴格的預設，故不列入必要 section。
const readAllowSection = "read_allow"

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
	fileOwners, queryOwners := sections["files"], sections["queries"]

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

	// ADR-035: read 與 write 走同一個棘輪，判定式只差在 query 是不是 write。
	// ADR-032 §2 的四種關係裡沒有「直接呼叫別人的 query」這一項——取別人的事實
	// 要 import 對方的公開 Service API，所以跨 context read 是 write 漂移的 read 版。
	for _, side := range []struct {
		write bool
		// section 是本側的容忍清單，other 是另一側的——條目放錯段落時
		// 訊息要直接指出該搬去哪裡，否則讀的人得自己推。
		section, other  string
		verb, otherVerb string // 訊息裡的動詞；`%ss` 補成 writes／reads
	}{
		{write: true, section: "allow", other: readAllowSection, verb: "write", otherVerb: "read"},
		{write: false, section: readAllowSection, other: "allow", verb: "read", otherVerb: "write"},
	} {
		tolerated := sections[side.section]
		for _, name := range sortedKeys(queries) {
			if queries[name].write != side.write {
				continue
			}
			allowed := map[string]bool{}
			for _, context := range splitList(tolerated[name]) {
				allowed[context] = true
			}
			for _, site := range calls[name] {
				if site.context == owner(name) || allowed[site.context] {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"cross-context %s: %s is owned by %q but %q %ss it at %s",
					side.verb, name, owner(name), site.context, side.verb, site.path))
			}
		}

		// A tolerated drift whose call site is gone must be deleted from the file;
		// leaving it there quietly re-opens the hole for the next caller.
		for _, name := range sortedKeys(tolerated) {
			query, ok := queries[name]
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"db/%s: %s.%s is not a query in db/queries", queryOwnersFile, side.section, name))
				continue
			}
			for _, context := range splitList(tolerated[name]) {
				switch {
				case !platformContexts[context]:
					problems = append(problems, fmt.Sprintf(
						"db/%s: %s.%s = %q is not a context in ADR-032 §1", queryOwnersFile, side.section, name, context))
				case query.write != side.write:
					problems = append(problems, fmt.Sprintf(
						"db/%s: %s.%s is a %s query; declare it in %s:",
						queryOwnersFile, side.section, name, side.otherVerb, side.other))
				case context == owner(name):
					problems = append(problems, fmt.Sprintf(
						"db/%s: %s.%s = %q is the owner; the entry is redundant", queryOwnersFile, side.section, name, context))
				case !callsFrom(calls[name], context):
					problems = append(problems, fmt.Sprintf(
						"db/%s: %s.%s = %q no longer calls it; delete the entry", queryOwnersFile, side.section, name, context))
				}
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

// ── ADR-032 §1 對照表的完整性 ──────────────────────────────────────────────
//
// AGENTS.md 第 11 條要求「新增套件必須先在 ADR-032 §1 對照表登記」，在此之前
// 沒有任何東西強制它——漏登記的套件會安靜地活在 internal/ 底下，既不屬於任何
// context，也不受 depguard 約束。這道檢查讓三份清單互相對帳：
//
//	apps/platform/internal/ 的套件目錄
//	ADR-032 §1 表格「internal/ 套件」欄
//	apps/platform/.golangci.yml 的 depguard 規則
//
// 三者任一方向缺漏都 FAIL，訊息指出是哪個套件、缺在哪一側。
//
// **depguard 覆蓋只對非 Generic 的列強制**。ADR-032 §1 的 Generic 列裡，
// `apiserver` 是 composition root（它 import 每一個 context，一條 deny 什麼都不能寫）、
// `api/gen` 是生成碼；兩者刻意沒有規則。其餘 Generic 套件（audit／outbox／
// llmclient／skillpkg／platform）實際上被 `generic` 那條規則覆蓋，這道檢查不反對——
// 它只要求「領域 context 一定要有人管」，不禁止 Generic 也被管。
const contextMapADR = "ADR-032-ddd-bounded-context-governance-for-platform.md"

var (
	// §1 的表格；下一個 `### ` 標題就是邊界。文件裡還有 §2、§5 與附錄 A 三張表，
	// 抓錯一張會讓這道檢查對著關係列表比對套件名。
	contextTableHeading = "### 1. Context 對照表"
	// 套件欄的儲存格會夾註解（`ingest`（…`SaveVersion`…）），註解裡也有反引號。
	// 先把全形括號夾住的部分整段拿掉，再抽反引號，否則 `SaveVersion` 會被當成套件。
	contextNotePattern    = regexp.MustCompile(`（[^（）]*）`)
	contextTokenPattern   = regexp.MustCompile("`([^`]+)`")
	contextPackagePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(/(\*|[a-z][a-z0-9_]*))?$`)
	// depguard 規則的 files: 清單，例如 `- "**/internal/registry/**"`。
	depguardFilePattern = regexp.MustCompile(`\*\*/internal/([a-z0-9_]+)/\*\*`)
)

func contextMapProblems(root string) []string {
	// 訊息裡的路徑一律用正斜線：它是給人看的 repo 相對路徑，不是要拿去開檔的。
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
	for _, match := range depguardFilePattern.FindAllStringSubmatch(string(lint), -1) {
		guarded[match[1]] = true
	}

	internal := filepath.Join(root, "apps", "platform", "internal")
	entries, err := os.ReadDir(internal)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform/internal: %v", err))
	}
	present := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			present[entry.Name()] = true
		}
	}

	for _, name := range sortedKeys(present) {
		if _, ok := declared[name]; !ok {
			problems = append(problems, fmt.Sprintf(
				"apps/platform/internal/%s is not listed in %s §1; register it before adding the package (AGENTS.md 第 11 條)",
				name, contextMapADR))
		}
	}
	for _, name := range sortedKeys(declared) {
		switch {
		case !present[name]:
			problems = append(problems, fmt.Sprintf(
				"%s §1 lists %q but apps/platform/internal/%s does not exist", contextMapADR, name, name))
		case !declared[name] && !guarded[name]: // 非 Generic 的列一定要有人管
			problems = append(problems, fmt.Sprintf(
				"%s §1 puts %q in a bounded context but %s has no depguard rule covering it",
				contextMapADR, name, lintPath))
		}
	}
	for _, name := range sortedKeys(guarded) {
		if !present[name] {
			problems = append(problems, fmt.Sprintf(
				"%s guards apps/platform/internal/%s but that package does not exist", lintPath, name))
		}
	}
	return problems
}

// contextTablePackages 回傳 §1 表格宣告的套件目錄，value 為 true 表示該列是
// Generic（跨切面，非 context），據此決定要不要強制 depguard 覆蓋。
// `platform/*` 與 `api/gen` 這類寫法取第一段當目錄名；不符形狀的 token 直接報錯，
// 不靜默跳過——安靜跳過的解析器等於沒有這道檢查。
func contextTablePackages(path, relative string) (map[string]bool, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", relative, err)}
	}
	declared := map[string]bool{}
	var problems []string
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "### ") {
			inTable = strings.HasPrefix(line, contextTableHeading)
			continue
		}
		if !inTable || !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 3 {
			continue
		}
		kind := strings.TrimSpace(cells[1])
		if kind == "類型" || strings.Trim(kind, "- ") == "" {
			continue // 表頭與分隔列
		}
		generic := kind == "Generic"
		for _, token := range contextTokenPattern.FindAllStringSubmatch(
			contextNotePattern.ReplaceAllString(cells[2], ""), -1) {
			name := token[1]
			if !contextPackagePattern.MatchString(name) {
				problems = append(problems, fmt.Sprintf(
					"%s §1 lists %q, which is not a package directory this check can read", relative, name))
				continue
			}
			directory, _, _ := strings.Cut(name, "/")
			// 同一個套件出現在兩列時（`skillpkg` 既是 Core 的一員也被當
			// Shared Kernel），只要有一列是非 Generic 就照非 Generic 要求。
			shared := generic
			if seen, ok := declared[directory]; ok {
				shared = shared && seen
			}
			declared[directory] = shared
		}
	}
	return declared, problems
}
