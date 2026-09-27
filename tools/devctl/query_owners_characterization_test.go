package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACrossContextCallNamesTheCallersFileUnderItsSourceTree(t *testing.T) {
	t.Parallel()
	root := writeQueryOwnerFixture(t,
		decl("files:\n  runs.sql: run\nqueries:\nallow:\n"),
		map[string]string{"runs.sql": "-- name: GetRun :one\nSELECT * FROM runs WHERE id = $1;\n"},
		map[string]string{"eval": "func f(q Q) { q.GetRun(ctx) }"})
	problems := queryOwnerProblems(root)
	if len(problems) != 1 || !strings.HasSuffix(problems[0], `"eval" reads it at apps/platform/internal/eval/service.go`) {
		t.Fatalf("got %#v, want one problem ending with the caller's path under apps/platform/internal", problems)
	}
}

func TestAnOwnerDeclarationRejectsTheSameEntryTwiceInOneSection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), queryOwnersFile)
	declaration := "files:\n  runs.sql: run\n  runs.sql: eval\nqueries:\nallow:\nread_allow:\nimmutable:\nimmutable_allow:\n"
	if err := os.WriteFile(path, []byte(declaration), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseOwnerDeclaration(path); err == nil || !strings.Contains(err.Error(), "line 3: files.runs.sql declared twice") {
		t.Fatalf("a repeated entry silently replaced the first: %v", err)
	}
}

func TestATableDeclaredWithoutAnOwnerIsReported(t *testing.T) {
	t.Parallel()
	declaration := decl("files:\n  q.sql: run\nqueries:\nallow:\n") +
		"tables:\n  runs: run\n  skills: run\n  artifacts:\n"
	root := writeQueryOwnerFixture(t, declaration, map[string]string{"q.sql": "-- name: OwnRead :many\nSELECT id FROM runs;\n"}, nil)
	writeMigration(t, root, "0001_init.sql", tableOwnerMigration)
	if joined := strings.Join(queryOwnerProblems(root), "\n"); !strings.Contains(joined, "tables.artifacts names no owner") {
		t.Fatalf("an ownerless table was accepted:\n%s", joined)
	}
}
