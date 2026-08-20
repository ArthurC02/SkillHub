package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeQueryOwnerFixture lays out the three inputs the check reads: the
// declaration, the .sql files it declares, and the Go callers. The caller files
// carry the sqlc import because that is what marks a `.Name(` as a query call.
func writeQueryOwnerFixture(t *testing.T, declaration string, sql map[string]string, callers map[string]string) string {
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
	for name, contents := range sql {
		write("db/queries/"+name, contents)
	}
	for context, body := range callers {
		write("apps/platform/internal/"+context+"/service.go",
			"package "+context+"\n\nimport \"github.com/ArthurC02/skillhub/apps/platform/"+
				strings.TrimSuffix(genImportPath, "\"")+"\"\n\n"+body+"\n")
	}
	return root
}

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

	baseDeclaration := "files:\n  runs.sql: run\nqueries:\nallow:\n"

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
			name:        "foreign context reading is not enforced",
			declaration: baseDeclaration,
			callers:     map[string]string{"eval": "func f(q Q) { q.GetRun(ctx) }"},
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
			declaration: "files:\n  runs.sql: run\nqueries:\nallow:\n  CreateRun: eval\n",
			callers:     map[string]string{"eval": "func f(q Q) { q.CreateRun(ctx) }"},
		},
		{
			name:        "stale allow entry is reported",
			declaration: "files:\n  runs.sql: run\nqueries:\nallow:\n  CreateRun: eval\n",
			callers:     map[string]string{"run": "func f(q Q) { q.CreateRun(ctx) }"},
			want:        "allow.CreateRun = \"eval\" no longer calls it",
		},
		{
			name:        "undeclared sql file is reported",
			declaration: "files:\nqueries:\nallow:\n",
			want:        "db/queries/runs.sql has no default owner",
		},
		{
			name:        "declaration of a vanished query is reported",
			declaration: "files:\n  runs.sql: run\nqueries:\n  ListRuns: run\nallow:\n",
			want:        "queries.ListRuns is not a query in db/queries",
		},
		{
			name:        "unknown context name is reported",
			declaration: "files:\n  runs.sql: runz\nqueries:\nallow:\n",
			want:        `files.runs.sql = "runz" is not a context`,
		},
		{
			name:        "per-query override beats the file default",
			declaration: "files:\n  runs.sql: run\nqueries:\n  CreateRun: eval\nallow:\n",
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

func TestQueryOwnerProblemsIgnoresTestsAndNonImporters(t *testing.T) {
	t.Parallel()
	// A cross-context write in a _test.go file, or in a file that never imports
	// the sqlc package, is not a production data-access path. Counting either
	// would make the check fire on integration tests that drive fixtures.
	root := writeQueryOwnerFixture(t,
		"files:\n  runs.sql: run\nqueries:\nallow:\n",
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
