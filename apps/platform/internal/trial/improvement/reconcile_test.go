package eval

import (
	"slices"
	"testing"
)

func TestRecoveryPicksUpOnlyEvaluationsStillWaitingForTheJudge(t *testing.T) {
	if got := statusesAwaitingTheJudge(); !slices.Equal(got, []string{"pending"}) {
		t.Fatalf("recovered statuses = %v, want only pending", got)
	}
}
