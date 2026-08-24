package run

// The two properties of the happy-path accessor that cannot be shown from outside
// the package, because both are about the transition table itself: that its shape
// no longer decides where a successful run goes, and that a table which cannot
// arrive at `succeeded` produces an error instead of a loop. Both swap `successors`
// for the duration of one test and put it back.

import (
	"errors"
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// swapSuccessors installs a table for one test. Nothing here runs in parallel, and
// the restore is what keeps that true for whatever runs next.
func swapSuccessors(t *testing.T, table map[gen.RunStatus][]gen.RunStatus) {
	t.Helper()
	original := successors
	successors = table
	t.Cleanup(func() { successors = original })
}

// Reordering a row is an edit that changes no legality whatsoever, and it used to
// change where settle took a successful run — the driver read element [0]. Reverse
// every row and the happy path must come out identical.
func TestRowOrderDoesNotDecideTheHappyPath(t *testing.T) {
	want, err := HappyPath(gen.RunStatusQueued)
	if err != nil {
		t.Fatalf("HappyPath(queued): %v", err)
	}

	reversed := make(map[gen.RunStatus][]gen.RunStatus, len(successors))
	for from, tos := range successors {
		row := slices.Clone(tos)
		slices.Reverse(row)
		reversed[from] = row
	}
	swapSuccessors(t, reversed)

	got, err := HappyPath(gen.RunStatusQueued)
	if err != nil {
		t.Fatalf("HappyPath(queued) with the rows reversed: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("reversing every row changed the happy path: got %v, want %v", got, want)
	}
}

// A table whose successes-only route never reaches `succeeded` must be reported.
// The walk in settle commits a transition per step, so the failure mode this
// guards against is not a wrong answer, it is a job writing to the database
// forever. If the cap ever stops working, this test does not fail — it hangs until
// the package timeout, which is the same signal one level louder.
func TestAWalkThatCannotArriveIsBounded(t *testing.T) {
	unhappy := []gen.RunStatus{gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut}
	swapSuccessors(t, map[gen.RunStatus][]gen.RunStatus{
		// A cycle: each state's happy successor leads back to the other, so
		// `succeeded` is unreachable while every individual move stays legal.
		gen.RunStatusQueued:       append([]gen.RunStatus{gen.RunStatusProvisioning}, unhappy...),
		gen.RunStatusProvisioning: append([]gen.RunStatus{gen.RunStatusQueued}, unhappy...),
	})

	path, err := HappyPath(gen.RunStatusQueued)
	if !errors.Is(err, ErrNoHappyPath) {
		t.Fatalf("HappyPath(queued) on a cyclic table = %v, %v; want ErrNoHappyPath", path, err)
	}
	if path != nil {
		t.Errorf("a failed walk returned %v; a caller must get nothing to walk", path)
	}
}
