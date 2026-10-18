package batch

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAPartlyMetTrialIsJudgedButNotPassed(t *testing.T) {
	testCase := distinctTestCases(1)[0]
	resolved := map[pgtype.UUID]resolvedTrial{}
	var group []gen.EvaluationBatchItem
	for i, outcome := range []TrialOutcome{TrialMet, TrialPartiallyMet, TrialNotMet} {
		run := pgtype.UUID{Bytes: [16]byte{byte(i + 1), 7}, Valid: true}
		group = append(group, gen.EvaluationBatchItem{TestCaseID: testCase, Trial: int32(i + 1), RunID: run})
		resolved[run] = resolvedTrial{end: ChainEnd{RunID: run}, outcome: outcome}
	}

	result := testCaseResult(group, resolved, 3)

	if result.Judged != 3 || result.Passed != 1 {
		t.Fatalf("met, partially met and not met: %d passed of %d judged, want 1 of 3", result.Passed, result.Judged)
	}
}
