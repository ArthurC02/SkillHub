package main

import (
	"strings"
	"testing"
)

func TestAQueryOrderingVersionsMustReadTheSkillsLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		body string
		want bool
	}{
		{"newest by ORDER BY on the table", "SELECT * FROM skill_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1;", true},
		{"highest number with max on the table", "SELECT max(v.version_number) FROM skill_versions v WHERE v.skill_id = $1;", true},
		{"a previous version by comparison", "SELECT id FROM skill_versions WHERE version_number < $1;", true},
		{"a later version joined on the table", "SELECT 1 FROM skills s JOIN skill_versions later ON later.skill_id = s.id WHERE later.version_number > $1;", true},
		{"newest read through the view", "SELECT * FROM skill_line_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1;", false},
		{"a table row picked from the view", "SELECT * FROM skill_versions WHERE id = (SELECT id FROM skill_line_versions ORDER BY version_number DESC LIMIT 1);", false},
		{"a version by id without ordering", "SELECT * FROM skill_versions WHERE id = $1 AND workspace_id = $2;", false},
		{"ordering another table", "SELECT * FROM bundle_versions ORDER BY version_number DESC;", false},
	} {
		if got := ordersVersionsOutsideLine(c.body); got != c.want {
			t.Errorf("%s: ordersVersionsOutsideLine = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOnlyANamedQueryMayOrderEveryVersion(t *testing.T) {
	t.Parallel()
	queries := map[string]sqlQuery{
		"Newest":  {file: "a.sql", ordersVersionsOutsideLine: true},
		"Listing": {file: "a.sql", ordersVersionsOutsideLine: true},
		"ByID":    {file: "a.sql"},
	}
	for _, c := range []struct {
		name    string
		allowed map[string]string
		want    []string
	}{
		{"an unnamed bypass", map[string]string{"Listing": "lists all"}, []string{"db/queries/a.sql: Newest orders skill_versions"}},
		{"every bypass named", map[string]string{"Listing": "lists all", "Newest": "x"}, nil},
		{"a name that no longer bypasses", map[string]string{"Listing": "lists all", "Newest": "x", "ByID": "y"}, []string{"names ByID, which no longer orders"}},
		{"a name for a query that is gone", map[string]string{"Listing": "lists all", "Newest": "x", "Gone": "z"}, []string{"names Gone, which no longer orders"}},
	} {
		got := lineBypasses(queries, c.allowed)
		if len(got) != len(c.want) {
			t.Fatalf("%s: problems = %v, want %d", c.name, got, len(c.want))
		}
		for i, fragment := range c.want {
			if !strings.Contains(got[i], fragment) {
				t.Errorf("%s: problem %q lacks %q", c.name, got[i], fragment)
			}
		}
	}
}
