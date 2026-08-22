package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const queryOwnerADRFixture = `### 1. Context 對照表

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| Skill 試跑執行／Run Orchestration | Core | run | run | RUN |
| 成果判定與改善／Evaluation & Improvement | Core | eval | eval | EVAL |
| Skill 收藏與版本歷史／Skill Registry & Versioning | Core | registry | registry | SKILL |
`

// writeQueryOwnerFixture lays out the three inputs the check reads: the
// declaration, the .sql files it declares, and the Go callers. The caller files
// carry the sqlc import because that is what marks a `.Name(` as a query call.
func writeQueryOwnerFixture(t *testing.T, declaration string, sql map[string]string, callers map[string]string) string {
	return writeQueryOwnerFixtureWithADR(t, queryOwnerADRFixture, declaration, sql, callers)
}

func writeQueryOwnerFixtureWithADR(t *testing.T, adr, declaration string, sql map[string]string, callers map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The scan root always exists in the repo; a missing one is a real error,
	// so the fixture creates it rather than teaching the check to shrug.
	if err := os.MkdirAll(filepath.Join(root, "apps", "platform", "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("db/"+queryOwnersFile, declaration)
	write("docs/adr/"+contextMapADR, adr)
	for name, contents := range sql {
		write("db/queries/"+name, contents)
	}
	for context, body := range callers {
		write("apps/platform/internal/"+context+"/service.go",
			"package "+filepath.Base(context)+"\n\nimport \"github.com/ArthurC02/skillhub/apps/platform/"+
				strings.TrimSuffix(genImportPath, "\"")+"\"\n\n"+body+"\n")
	}
	return root
}

// decl appends the two sections the ownership cases do not exercise. They are
// required to exist, so a declaration that omits them fails to parse before the
// case under test gets a chance to run.
func decl(declaration string) string { return declaration + "immutable:\nimmutable_allow:\n" }

func TestQueryOwnerProblems(t *testing.T) {
	t.Parallel()

	const queries = `-- name: CreateRun :one
INSERT INTO runs (id) VALUES ($1) RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1;

-- name: PurgeRuns :execrows
-- A CTE write: the body opens on a SELECT and still deletes.
WITH doomed AS (SELECT id FROM runs WHERE stale)
DELETE FROM runs WHERE id IN (SELECT id FROM doomed);
`

	baseDeclaration := decl("files:\n  runs.sql: run\nqueries:\nallow:\n")

	tests := []struct {
		name        string
		declaration string
		callers     map[string]string
		want        string // substring of the expected single problem; empty means clean
	}{
		{
			name:        "owner writes its own query",
			declaration: baseDeclaration,
			callers:     map[string]string{"run": "func f(q Q) { q.CreateRun(ctx) }"},
		},
		{
			name:        "owner reads its own query",
			declaration: baseDeclaration,
			callers:     map[string]string{"run": "func f(q Q) { q.GetRun(ctx) }"},
		},
		{
			name:        "another package in the owner context may read",
			declaration: baseDeclaration,
			callers:     map[string]string{"run/read": "func f(q Q) { q.GetRun(ctx) }"},
		},
		{
			name:        "foreign context reading is blocked",
			declaration: baseDeclaration,
			callers:     map[string]string{"eval": "func f(q Q) { q.GetRun(ctx) }"},
			want:        `GetRun is owned by "run" but "eval" reads it`,
		},
		{
			name:        "read_allow entry lets a known drift through",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\nread_allow:\n  GetRun: eval\n"),
			callers:     map[string]string{"eval": "func f(q Q) { q.GetRun(ctx) }"},
		},
		{
			name:        "stale read_allow entry is reported",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\nread_allow:\n  GetRun: eval\n"),
			callers:     map[string]string{"run": "func f(q Q) { q.GetRun(ctx) }"},
			want:        "read_allow.GetRun = \"eval\" no longer calls it",
		},
		{
			name:        "a write parked in read_allow is sent to the other section",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\nread_allow:\n  CreateRun: eval\n"),
			want:        "read_allow.CreateRun is a write query; declare it in allow:",
		},
		{
			name:        "a read parked in allow is sent to the other section",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\n  GetRun: eval\nread_allow:\n"),
			want:        "allow.GetRun is a read query; declare it in read_allow:",
		},
		{
			name:        "read_allow entry for a vanished query is reported",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\nread_allow:\n  ListRuns: eval\n"),
			want:        "read_allow.ListRuns is not a query in db/queries",
		},
		{
			name:        "foreign context writing is blocked",
			declaration: baseDeclaration,
			callers:     map[string]string{"eval": "func f(q Q) { q.CreateRun(ctx) }"},
			want:        `CreateRun is owned by "run" but "eval" writes it`,
		},
		{
			name:        "CTE write is recognised as a write",
			declaration: baseDeclaration,
			callers:     map[string]string{"eval": "func f(q Q) { q.PurgeRuns(ctx) }"},
			want:        `PurgeRuns is owned by "run" but "eval" writes it`,
		},
		{
			name:        "allow entry lets a known drift through",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\n  CreateRun: eval\n"),
			callers:     map[string]string{"eval": "func f(q Q) { q.CreateRun(ctx) }"},
		},
		{
			name:        "stale allow entry is reported",
			declaration: decl("files:\n  runs.sql: run\nqueries:\nallow:\n  CreateRun: eval\n"),
			callers:     map[string]string{"run": "func f(q Q) { q.CreateRun(ctx) }"},
			want:        "allow.CreateRun = \"eval\" no longer calls it",
		},
		{
			name:        "undeclared sql file is reported",
			declaration: decl("files:\nqueries:\nallow:\n"),
			want:        "db/queries/runs.sql has no default owner",
		},
		{
			name:        "declaration of a vanished query is reported",
			declaration: decl("files:\n  runs.sql: run\nqueries:\n  ListRuns: run\nallow:\n"),
			want:        "queries.ListRuns is not a query in db/queries",
		},
		{
			name:        "unknown context name is reported",
			declaration: decl("files:\n  runs.sql: runz\nqueries:\nallow:\n"),
			want:        `files.runs.sql = "runz" is not a Boundary ID`,
		},
		{
			name:        "unregistered caller is reported",
			declaration: baseDeclaration,
			callers:     map[string]string{"billing": "func f(q Q) { q.GetRun(ctx) }"},
			want:        `apps/platform/internal/billing calls sqlc but has no architecture identity`,
		},
		{
			name:        "per-query override beats the file default",
			declaration: decl("files:\n  runs.sql: run\nqueries:\n  CreateRun: eval\nallow:\n"),
			callers:     map[string]string{"eval": "func f(q Q) { q.CreateRun(ctx) }"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := writeQueryOwnerFixture(t, test.declaration, map[string]string{"runs.sql": queries}, test.callers)
			problems := queryOwnerProblems(root)
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %#v", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("expected exactly one problem containing %q, got %#v", test.want, problems)
			}
			if !strings.Contains(problems[0], test.want) {
				t.Fatalf("problem %q does not mention %q", problems[0], test.want)
			}
		})
	}
}

func TestQueryOwnerProblemsNestedCallerUsesBoundaryID(t *testing.T) {
	t.Parallel()
	adr := strings.Replace(queryOwnerADRFixture, "| run | run | RUN |", "| run | trial/execution | RUN |", 1)
	adr = strings.Replace(adr, "| eval | eval | EVAL |", "| eval | trial/evidence | EVAL |", 1)
	root := writeQueryOwnerFixtureWithADR(t, adr,
		decl("files:\n  runs.sql: run\nqueries:\nallow:\n"),
		map[string]string{"runs.sql": "-- name: CreateRun :one\nINSERT INTO runs (id) VALUES ($1);\n"},
		map[string]string{"trial/evidence": "func f(q Q) { q.CreateRun(ctx) }"})
	problems := queryOwnerProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], `CreateRun is owned by "run" but "eval" writes it`) {
		t.Fatalf("expected stable Boundary IDs in nested ownership error, got %#v", problems)
	}
}

func TestQueryOwnerProblemsIgnoresTestsAndNonImporters(t *testing.T) {
	t.Parallel()
	// A cross-context write in a _test.go file, or in a file that never imports
	// the sqlc package, is not a production data-access path. Counting either
	// would make the check fire on integration tests that drive fixtures.
	root := writeQueryOwnerFixture(t,
		decl("files:\n  runs.sql: run\nqueries:\nallow:\n"),
		map[string]string{"runs.sql": "-- name: CreateRun :one\nINSERT INTO runs (id) VALUES ($1);\n"},
		nil)
	for _, relative := range []string{"eval/service_test.go", "eval/helper.go"} {
		path := filepath.Join(root, "apps", "platform", "internal", filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package eval\n\nfunc f(q Q) { q.CreateRun(ctx) }\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if problems := queryOwnerProblems(root); len(problems) != 0 {
		t.Fatalf("expected no problems, got %#v", problems)
	}
}

func TestImmutableTableProblems(t *testing.T) {
	t.Parallel()

	// The migration half of the contract. `notes` is deliberately frozen only
	// while draft, which is what the check must refuse to treat as a frozen
	// table: a column-scoped or conditional trigger says "part of this row",
	// not "this table is insert-only".
	const migration = `
CREATE TRIGGER skill_versions_immutable
    BEFORE UPDATE OR DELETE ON skill_versions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER notes_immutable
    BEFORE UPDATE OR DELETE ON notes
    FOR EACH ROW WHEN (OLD.status = 'draft')
    EXECUTE FUNCTION enforce_immutable();
`
	const frozen = "immutable:\n  skill_versions: ADR-003\n"

	tests := []struct {
		name    string
		queries string
		suffix  string // the immutable / immutable_allow sections
		want    string
	}{
		{
			name:    "insert into a frozen table is fine",
			queries: "-- name: CreateSkillVersion :one\nINSERT INTO skill_versions (id) VALUES ($1);\n",
			suffix:  frozen + "immutable_allow:\n",
		},
		{
			name:    "update of a frozen table is blocked",
			queries: "-- name: TouchVersion :exec\nUPDATE skill_versions SET manifest = $2 WHERE id = $1;\n",
			suffix:  frozen + "immutable_allow:\n",
			want:    "skill_versions is append-only but TouchVersion updates or deletes it",
		},
		{
			name: "delete from a frozen table is blocked, CTE included",
			queries: "-- name: PurgeVersions :execrows\n" +
				"WITH doomed AS (SELECT id FROM skills)\nDELETE FROM skill_versions WHERE skill_id IN (SELECT id FROM doomed);\n",
			suffix: frozen + "immutable_allow:\n",
			want:   "skill_versions is append-only but PurgeVersions updates or deletes it",
		},
		{
			name:    "named exemption lets the retention purge through",
			queries: "-- name: PurgeVersions :execrows\nDELETE FROM skill_versions WHERE stale;\n",
			suffix:  frozen + "immutable_allow:\n  PurgeVersions: skill_versions\n",
		},
		{
			name:    "exemption whose statement no longer writes the table is reported",
			queries: "-- name: PurgeVersions :execrows\nDELETE FROM skills WHERE stale;\n",
			suffix:  frozen + "immutable_allow:\n  PurgeVersions: skill_versions\n",
			want:    `immutable_allow.PurgeVersions = "skill_versions" no longer writes it`,
		},
		{
			name:    "exemption for a table nobody declared immutable is reported",
			queries: "-- name: PurgeSkills :execrows\nDELETE FROM skills WHERE stale;\n",
			suffix:  frozen + "immutable_allow:\n  PurgeSkills: skills\n",
			want:    `immutable_allow.PurgeSkills = "skills" is not a declared immutable table`,
		},
		{
			name:    "declaring a table the database does not freeze is reported",
			queries: "-- name: CreateSkillVersion :one\nINSERT INTO skill_versions (id) VALUES ($1);\n",
			suffix:  "immutable:\n  notes: conditional, not a frozen table\nimmutable_allow:\n",
			want:    "immutable.notes has no unconditional enforce_immutable() trigger",
		},
		{
			name:    "declaring a table without a reason is reported",
			queries: "-- name: CreateSkillVersion :one\nINSERT INTO skill_versions (id) VALUES ($1);\n",
			suffix:  "immutable:\n  skill_versions:\nimmutable_allow:\n",
			want:    "immutable.skill_versions has no reason",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := writeQueryOwnerFixture(t,
				"files:\n  versions.sql: registry\nqueries:\nallow:\n"+test.suffix,
				map[string]string{"versions.sql": test.queries}, nil)
			path := filepath.Join(root, "db", "migrations", "0005_immutability.sql")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(migration), 0o600); err != nil {
				t.Fatal(err)
			}
			problems := queryOwnerProblems(root)
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %#v", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("expected exactly one problem containing %q, got %#v", test.want, problems)
			}
			if !strings.Contains(problems[0], test.want) {
				t.Fatalf("problem %q does not mention %q", problems[0], test.want)
			}
		})
	}
}

func TestMutatedTables(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"SELECT * FROM skill_versions FOR UPDATE":                      nil,
		"INSERT INTO skill_versions (id) VALUES ($1)":                  nil,
		"INSERT INTO skills VALUES ($1) ON CONFLICT DO UPDATE SET a=1": nil,
		"UPDATE skill_versions SET a = 1":                              {"skill_versions"},
		"DELETE FROM   ONLY skill_versions WHERE id = $1":              {"skill_versions"},
		"WITH x AS (DELETE FROM a) UPDATE b SET c = 1":                 {"a", "b"},
	}
	for body, want := range tests {
		if got := mutatedTables(body); !slices.Equal(got, want) {
			t.Errorf("mutatedTables(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestIsWriteStatement(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"SELECT * FROM runs WHERE id = $1":                           false,
		"SELECT * FROM runs WHERE id = $1 FOR UPDATE":                false,
		"SELECT * FROM runs FOR NO KEY UPDATE":                       false,
		"-- deletes nothing, just documents DELETE\nSELECT 1":        false,
		"SELECT * FROM audit WHERE action = 'delete'":                false,
		"UPDATE runs SET updated_at = now()":                         true,
		"WITH x AS (SELECT id FROM runs) DELETE FROM runs":           true,
		"INSERT INTO runs VALUES ($1) ON CONFLICT DO UPDATE SET a=1": true,
	}
	for body, want := range tests {
		if got := isWriteStatement(body); got != want {
			t.Errorf("isWriteStatement(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestRawSQLProblems(t *testing.T) {
	t.Parallel()

	// These flat internal/<boundary-id>/ paths are deliberate legacy-layout fixtures.
	// They verify the parser continues to diagnose repositories before nested migration;
	// do not rewrite them to the current product-address paths.
	tests := []struct {
		name  string
		path  string // relative to apps/platform/
		body  string
		allow map[string]string
		want  string // substring of the expected single problem; empty means clean
	}{
		{
			name: "sqlc call is clean",
			path: "internal/run/service.go",
			body: "func f(q Q) { q.CreateRun(ctx) }",
		},
		{
			name: "raw UPDATE is blocked",
			path: "internal/run/service.go",
			body: `func f(tx T) { tx.Exec(ctx, "UPDATE skills SET name = 'x'") }`,
			want: `internal/run/service.go:3 (f) passes "UPDATE skills SET name = 'x'" to Exec`,
		},
		{
			name: "raw DELETE is blocked",
			path: "internal/eval/purge.go",
			body: "func f(tx T) { tx.Exec(ctx, `DELETE FROM runs WHERE id = $1`, id) }",
			want: `passes "DELETE FROM runs WHERE id = $1" to Exec`,
		},
		{
			name: "raw INSERT is blocked",
			path: "cmd/worker/main.go",
			body: `func f(b B) { b.Queue("INSERT INTO audit_events (id) VALUES ($1)", id) }`,
			want: `apps/platform/cmd/worker/main.go:3 (f) passes "INSERT INTO audit_events (id) VALUES ($1)" to Queue`,
		},
		{
			name: "raw DML in a _test.go file is not constrained",
			path: "internal/run/service_test.go",
			body: `func f(tx T) { tx.Exec(ctx, "UPDATE skills SET name = 'x'") }`,
		},
		{
			name: "raw DML in a generated directory is not constrained",
			path: "internal/foundation/persistence/db/gen/queries.sql.go",
			body: `func f(tx T) { tx.Exec(ctx, "UPDATE skills SET name = 'x'") }`,
		},
		{
			name: "raw SELECT is blocked",
			path: "internal/eval/reconcile.go",
			body: "func f(p P) { p.Query(ctx, `SELECT id FROM evaluations WHERE status = 'pending'`) }",
			want: `passes "SELECT id FROM evaluations WHERE status = 'pending'" to Query`,
		},
		{
			name: "package const is resolved",
			path: "internal/run/halt.go",
			body: "const statement = `SELECT id FROM river_job`\nfunc f(pool P) { pool.Query(ctx, statement) }",
			want: `passes "SELECT id FROM river_job" to Query`,
		},
		{
			name: "function local is resolved",
			path: "internal/foundation/persistence/partition/partition.go",
			body: "func f(pool P) { statement := `CREATE TABLE child (id int)`; pool.Exec(ctx, statement) }",
			want: `passes "CREATE TABLE child (id int)" to Exec`,
		},
		{
			name: "fmt Sprintf format is resolved",
			path: "internal/foundation/persistence/partition/partition.go",
			body: `func f(pool P) { pool.Exec(ctx, fmt.Sprintf("DROP TABLE %s", name)) }`,
			want: `passes "DROP TABLE %s" to Exec`,
		},
		{
			name: "WITH is treated as SQL",
			path: "internal/run/service.go",
			body: `func f(tx T) { tx.Exec(ctx, "WITH live AS (SELECT 1) SELECT * FROM live") }`,
			want: `passes "WITH live AS (SELECT 1) SELECT * FROM live" to Exec`,
		},
		{
			name:  "function-scoped exemption lets known SQL through",
			path:  "internal/packaging/packaging.go",
			body:  `func f(tx T) { tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", k) }`,
			allow: map[string]string{"apps/platform/internal/packaging/packaging.go@f": "serialize package persistence"},
		},
		{
			name:  "an exemption does not cover another function in the file",
			path:  "internal/run/service.go",
			body:  "func f(tx T) { tx.Exec(ctx, \"SELECT 1\") }\nfunc g(tx T) { tx.Exec(ctx, \"DELETE FROM skills\") }",
			allow: map[string]string{"apps/platform/internal/run/service.go@f": "technical probe"},
			want:  `internal/run/service.go:4 (g) passes "DELETE FROM skills" to Exec`,
		},
		{
			name:  "exemption without a reason is reported",
			path:  "internal/run/service.go",
			body:  `func f(tx T) { tx.Exec(ctx, "UPDATE skills SET name = 'x'") }`,
			allow: map[string]string{"apps/platform/internal/run/service.go@f": ""},
			want:  "raw_sql_allow.apps/platform/internal/run/service.go@f has no reason",
		},
		{
			name:  "stale exemption is reported",
			path:  "internal/run/service.go",
			body:  "func f(q Q) { q.CreateRun(ctx) }",
			allow: map[string]string{"apps/platform/internal/run/service.go@f": "technical query"},
			want:  "raw_sql_allow.apps/platform/internal/run/service.go@f no longer contains raw SQL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "apps", "platform", filepath.FromSlash(test.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			source := "package p\n\n" + test.body + "\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			problems := rawSQLProblems(root, test.allow)
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %#v", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("expected exactly one problem containing %q, got %#v", test.want, problems)
			}
			if !strings.Contains(problems[0], test.want) {
				t.Fatalf("problem %q does not mention %q", problems[0], test.want)
			}
		})
	}
}
