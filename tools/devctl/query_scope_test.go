package main

import (
	"strings"
	"testing"
)

func TestTheRealRepositoryDeclaresEveryUnscopedQuery(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := queryScopeProblems(root); len(problems) > 0 {
		t.Fatalf("queries without a workspace condition are undeclared or stale:\n%s", strings.Join(problems, "\n"))
	}
}

func TestLacksWorkspaceCondition(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		body string
		want bool
	}{
		{"positional parameter, no workspace", "SELECT * FROM users WHERE id = $1;", true},
		{"named parameter, no workspace", "SELECT * FROM users WHERE id = @user_id;", true},
		{"sqlc.arg, no workspace", "SELECT * FROM users WHERE id = sqlc.arg(user_id);", true},
		{"sqlc.narg, no workspace", "SELECT * FROM users WHERE email = sqlc.narg(email);", true},
		{"sqlc.slice, no workspace", "SELECT * FROM users WHERE id = ANY(sqlc.slice(ids));", true},
		{"workspace condition", "SELECT * FROM skills WHERE id = $1 AND workspace_id = $2;", false},
		{"workspace condition in upper case", "SELECT * FROM skills WHERE ID = $1 AND WORKSPACE_ID = $2;", false},
		{"no parameter", "SELECT count(*) FROM users;", false},
		{"workspace named only in a comment", "-- filters by workspace_id upstream\nSELECT * FROM users WHERE id = $1;", true},
		{"parameter only in a comment", "-- was WHERE id = $1\nSELECT count(*) FROM users;", false},
		{"at sign only inside a literal", "SELECT count(*) FROM users WHERE email LIKE '%@example.com';", false},
		{"full-text match operator", "SELECT count(*) FROM search_documents WHERE document @@to_tsquery('x');", false},
		{"jsonb containment operator", "SELECT count(*) FROM runs WHERE meta @> '{}'::jsonb;", false},
		{"advisory lock that touches no table", "SELECT pg_advisory_xact_lock(hashtextextended(@lock_key::text, 0));", false},
		{"write without workspace", "UPDATE users SET display_name = @name WHERE id = @id;", true},
		{"workspace only in the select list", "SELECT id, workspace_id FROM runs WHERE id = $1;", true},
		{"workspace only counted", "SELECT count(DISTINCT workspace_id) FROM runs WHERE created_at >= @since;", true},
		{"workspace only returned by a write", "UPDATE runs SET seen = true WHERE id = @id RETURNING workspace_id;", true},
		{"workspace parameter matched against the workspace key", "SELECT owner_user_id FROM workspaces w WHERE w.id = sqlc.arg(workspace_id)::uuid;", false},
		{"workspace compared on the right", "SELECT * FROM skills s WHERE @owner = s.workspace_id;", false},
		{"workspace in a tuple comparison", "SELECT * FROM skills WHERE (id, workspace_id) = ($1, $2);", false},
		{"workspace in a list", "SELECT * FROM skills WHERE workspace_id IN (SELECT id FROM workspaces WHERE owner_user_id = $1);", false},
		{"insert that sets the workspace", "INSERT INTO skills (workspace_id, name) VALUES ($1, $2);", false},
		{"workspace only joined column to column", "SELECT * FROM runs r JOIN skills s ON s.workspace_id = r.workspace_id WHERE r.id = $1;", true},
		{"joined and filtered by the workspace", "SELECT * FROM runs r JOIN skills s ON s.workspace_id = r.workspace_id WHERE r.id = $1 AND r.workspace_id = $2;", false},
		{"a union branch without the workspace", "SELECT id FROM skills WHERE workspace_id = $1 UNION ALL SELECT id FROM runs WHERE id = $2;", true},
		{"every union branch scoped", "SELECT id FROM skills WHERE workspace_id = $1 UNION SELECT id FROM runs WHERE workspace_id = $1;", false},
		{"workspace named only in a block comment", "/* WHERE workspace_id = $2 */ SELECT * FROM users WHERE id = $1;", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := lacksWorkspaceCondition(c.body); got != c.want {
				t.Errorf("lacksWorkspaceCondition(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

const scopeFixtureSQL = `-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetSkill :one
SELECT * FROM skills WHERE id = $1 AND workspace_id = $2;

-- name: CountUsers :one
SELECT count(*) FROM users;
`

func scopeProblems(t *testing.T, scope string) []string {
	t.Helper()
	root := writeQueryOwnerFixture(t,
		decl("files:\n  a.sql: run\nqueries:\nallow:\n"+scope),
		map[string]string{"a.sql": scopeFixtureSQL}, nil)
	return queryScopeProblems(root)
}

func TestQueryScopeAcceptsADeclaredUnscopedQuery(t *testing.T) {
	t.Parallel()
	if problems := scopeProblems(t, "scope:\n  GetUser: user\n"); len(problems) != 0 {
		t.Fatalf("a declared unscoped query and an undeclared scoped one were refused: %v", problems)
	}
}

func TestQueryScopeProblems(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		scope string
		want  string
	}{
		{"section missing", "", `missing section "scope"`},
		{"unscoped query undeclared", "scope:\n", "db/queries/a.sql: GetUser takes parameters but never names workspace_id"},
		{"value outside the vocabulary", "scope:\n  GetUser: everyone\n", `scope.GetUser = "everyone" is not one of user, operator, worker, content-addressed, scoped-upstream, platform-wide`},
		{"declared query now has a workspace condition", "scope:\n  GetUser: user\n  GetSkill: user\n", "scope.GetSkill is declared, but db/queries/a.sql now names workspace_id"},
		{"declared query takes no parameter", "scope:\n  GetUser: user\n  CountUsers: operator\n", "scope.CountUsers is declared, but db/queries/a.sql now names workspace_id or takes no parameter"},
		{"declared query does not exist", "scope:\n  GetUser: user\n  GetGhost: worker\n", "scope.GetGhost is not a query in db/queries"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			problems := scopeProblems(t, c.scope)
			if len(problems) != 1 || !strings.Contains(problems[0], c.want) {
				t.Fatalf("problems = %v, want exactly one containing %q", problems, c.want)
			}
		})
	}
}
