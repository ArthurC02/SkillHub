package main

import (
	"strings"
	"testing"
)

func TestTheRealRepositoryKeepsItsSQLLogicAtTheBaseline(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := sqlLogicProblems(root); len(problems) > 0 {
		t.Fatalf("SQL logic moved off its baseline:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSQLLogicCountsDecisionsButNotComments(t *testing.T) {
	t.Parallel()
	body := `-- CASE WHEN COALESCE( IN ('x') interval '1 day'
SELECT case when a then 1 end, coalesce (b, 0), Coalesce(c, 0)
FROM runs
WHERE status in ( 'queued', 'running') AND created_at > now() - INTERVAL '1 day'
  AND updated_at > now() - '2 hours'::interval AND id IN (SELECT id FROM x);`
	want := sqlLogic{cases: 1, coalesces: 2, literalLists: 1, intervals: 2}
	if got := sqlLogicOf(body); got != want {
		t.Fatalf("sqlLogicOf = %+v, want %+v", got, want)
	}
}

func TestTheSQLLogicRatchetOnlyLetsCountsFall(t *testing.T) {
	t.Parallel()
	query := func(logic sqlLogic) sqlQuery { return sqlQuery{file: "runs.sql", logic: logic} }
	for _, c := range []struct {
		name     string
		queries  map[string]sqlQuery
		baseline map[string]sqlLogic
		want     string
	}{
		{"at the baseline", map[string]sqlQuery{"Q": query(sqlLogic{cases: 1})}, map[string]sqlLogic{"Q": {cases: 1}}, ""},
		{"a new query without logic", map[string]sqlQuery{"Q": query(sqlLogic{})}, nil, ""},
		{"a new query with a CASE", map[string]sqlQuery{"Q": query(sqlLogic{cases: 1})}, nil, "more than its baseline"},
		{"one more COALESCE", map[string]sqlQuery{"Q": query(sqlLogic{coalesces: 2})}, map[string]sqlLogic{"Q": {coalesces: 1}}, "more than its baseline"},
		{"an interval literal where none was", map[string]sqlQuery{"Q": query(sqlLogic{cases: 0, intervals: 1})}, map[string]sqlLogic{"Q": {cases: 1}}, "more than its baseline"},
		{"a literal list traded for a CASE", map[string]sqlQuery{"Q": query(sqlLogic{literalLists: 1})}, map[string]sqlLogic{"Q": {cases: 1}}, "more than its baseline"},
		{"one fewer and the baseline not lowered", map[string]sqlQuery{"Q": query(sqlLogic{})}, map[string]sqlLogic{"Q": {cases: 1}}, "lower its baseline"},
		{"a baseline for a removed query", map[string]sqlQuery{}, map[string]sqlLogic{"Gone": {cases: 1}}, "drop its baseline"},
	} {
		t.Run(c.name, func(t *testing.T) {
			problems := strings.Join(sqlLogicRatchet(c.queries, c.baseline), "\n")
			if c.want == "" && problems != "" || c.want != "" && !strings.Contains(problems, c.want) {
				t.Fatalf("problems = %q, want %q", problems, c.want)
			}
		})
	}
}
