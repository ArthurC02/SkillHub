package run_test

import (
	"slices"
	"testing"

	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestEvidenceAcceptsEveryRunStatusAsALifecycleTarget(t *testing.T) {
	var statuses []string
	for _, s := range run.AllStatuses {
		statuses = append(statuses, string(s))
	}
	slices.Sort(statuses)
	accepted := slices.Sorted(slices.Values(trace.RunLifecycleStatuses))
	if !slices.Equal(statuses, accepted) {
		t.Errorf("run_lifecycle to_status accepts %v, want every run status %v", accepted, statuses)
	}
}
