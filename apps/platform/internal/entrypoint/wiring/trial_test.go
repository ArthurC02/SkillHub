package wiring

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestEvaluationTreatsARunAsFinishedExactlyInItsFourEndStates(t *testing.T) {
	for _, tc := range []struct {
		status   gen.RunStatus
		finished bool
	}{
		{gen.RunStatusQueued, false},
		{gen.RunStatusProvisioning, false},
		{gen.RunStatusPreparing, false},
		{gen.RunStatusRunning, false},
		{gen.RunStatusEvaluating, false},
		{gen.RunStatusSucceeded, true},
		{gen.RunStatusFailed, true},
		{gen.RunStatusCancelled, true},
		{gen.RunStatusTimedOut, true},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := evalRunFacts(run.EvaluationRun{Status: string(tc.status)}).Terminal; got != tc.finished {
				t.Errorf("evaluation sees %s as finished = %v, want %v", tc.status, got, tc.finished)
			}
		})
	}
}
