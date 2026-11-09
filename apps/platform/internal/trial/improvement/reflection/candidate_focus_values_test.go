package reflection

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestTheCandidateFocusesAreTheValuesTheTableAccepts(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "db", "migrations", "0150_reflection_candidates.sql"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`candidate_focuses <@ ARRAY\[([^\]]*)\]`).FindAllSubmatch(body, -1)
	if len(found) != 1 {
		t.Fatalf("%d focus checks in the table, want exactly one", len(found))
	}
	var table []string
	for _, v := range regexp.MustCompile(`'([^']*)'`).FindAllSubmatch(found[0][1], -1) {
		table = append(table, string(v[1]))
	}
	got := slices.Clone(candidateFocuses[:])
	slices.Sort(got)
	slices.Sort(table)
	if !slices.Equal(got, table) {
		t.Errorf("Go focuses %v, the table accepts %v", got, table)
	}
}
