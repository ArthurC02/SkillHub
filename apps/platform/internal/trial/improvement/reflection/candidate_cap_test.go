package reflection

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/evalsettings"
)

func migrationBound(t *testing.T, file, pattern string) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "db", "migrations", file))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(pattern).FindAllSubmatch(body, -1)
	if len(found) != 1 {
		t.Fatalf("%s: %d matches for %q, want exactly one", file, len(found), pattern)
	}
	n, err := strconv.Atoi(string(found[0][1]))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEveryStatementOfTheCandidateCapAgreesWithTheOperatorCap(t *testing.T) {
	limit := evalsettings.MaxReflectionCandidates
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"one focus per candidate", len(candidateFocuses), limit},
		{"the contract's operator count", limits.SetReflectionCandidatesBodyCountMaximum, limit},
		{"the contract's listed candidates", limits.BatchReflectionCandidatesMaxItems, limit},
		{"the operator setting check", migrationBound(t, "0150_reflection_candidates.sql",
			`reflection_candidates BETWEEN 1 AND (\d+)`), limit},
		{"the round's recorded focuses", migrationBound(t, "0150_reflection_candidates.sql",
			`cardinality\(candidate_focuses\) <= (\d+)`), limit},
		{"a round suggestion's candidate index", migrationBound(t, "0150_reflection_candidates.sql",
			`candidate_range CHECK \(candidate BETWEEN 0 AND (\d+)\)`), limit - 1},
		{"a candidate batch's index", migrationBound(t, "0151_candidate_evaluation.sql",
			`candidate +smallint NOT NULL CHECK \(candidate BETWEEN 0 AND (\d+)\)`), limit - 1},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d to match the operator cap of %d", tc.name, tc.got, tc.want, limit)
		}
	}
}
