package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestTheRealRepositoryMakesNoDecisionsInSQL(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := sqlLogicProblems(root); len(problems) > 0 {
		t.Fatalf("queries decide in SQL:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSQLDecisionsAreDataBranchesLiteralListsAndIntervalsButNotMechanisms(t *testing.T) {
	t.Parallel()
	const (
		dataCase     = "a CASE branching on data"
		literalList  = "a literal IN list"
		intervalLit  = "an interval literal"
		commentedOut = "-- CASE WHEN a THEN 1 END, IN ('x'), interval '1 day'\nSELECT 1;"
	)
	for _, c := range []struct {
		name string
		body string
		want []string
	}{
		{"constructs inside comments", commentedOut, nil},
		{"zero values and write-once stamps", "SELECT coalesce(sum(n), 0) FROM t; UPDATE t SET at = COALESCE(at, now());", nil},
		{"a CASE applying a named parameter", "SET at = CASE WHEN sqlc.arg(restart)::bool THEN NULL ELSE at END", nil},
		{"a CASE applying a nullable parameter", "SET at = case when sqlc.narg( restart ) then null else at end", nil},
		{"a CASE applying an @ parameter", "SET at = CASE WHEN @restart::bool THEN NULL ELSE at END", nil},
		{"a CASE applying a positional parameter", "SET at = CASE WHEN $2 THEN NULL ELSE at END", nil},
		{"a searched CASE on a column", "SELECT case when a > 1 then 1 end FROM t", []string{dataCase}},
		{"a simple CASE on a column", "SELECT CASE status WHEN 'queued' THEN 1 END FROM t", []string{dataCase}},
		{"a parameter joined with data", "SELECT CASE WHEN sqlc.arg(a)::bool AND b THEN 1 END FROM t", []string{dataCase}},
		{"a data branch after a parameter branch", "SELECT CASE WHEN @a THEN 1 WHEN b THEN 2 END FROM t", []string{dataCase}},
		{"a literal list", "WHERE status in ( 'queued', 'running')", []string{literalList}},
		{"a subquery list", "WHERE id IN (SELECT id FROM x)", nil},
		{"an interval keyword literal", "WHERE at > now() - INTERVAL '1 day'", []string{intervalLit}},
		{"an interval cast literal", "WHERE at > now() - '2 hours'::interval", []string{intervalLit}},
		{"an interval parameter", "WHERE at > now() - @lease::interval", nil},
		{"every construct at once", "SELECT CASE WHEN a THEN 1 END FROM t WHERE s IN ('x') AND at > now() - interval '1 day'", []string{dataCase, literalList, intervalLit}},
	} {
		if got := sqlDecisionsOf(c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: decisions = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestADecidingQueryIsNamedWithItsFileAndConstructsAndACleanOneIsNot(t *testing.T) {
	t.Parallel()
	problems := sqlDecisionProblems(map[string]sqlQuery{
		"Clean":         {file: "runs.sql"},
		"DecidingOnce":  {file: "search.sql", decisions: []string{"an interval literal"}},
		"DecidingTwice": {file: "trace.sql", decisions: []string{"a CASE branching on data", "a literal IN list"}},
	})
	want := []string{
		"db/queries/search.sql: DecidingOnce decides in SQL with an interval literal; decide in Go and pass the result as a parameter",
		"db/queries/trace.sql: DecidingTwice decides in SQL with a CASE branching on data, a literal IN list; decide in Go and pass the result as a parameter",
	}
	if !reflect.DeepEqual(problems, want) {
		t.Fatalf("problems = %q, want %q", problems, want)
	}
}
