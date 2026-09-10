package run

import (
	"errors"
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func swapSuccessors(t *testing.T, table map[gen.RunStatus][]gen.RunStatus) {
	t.Helper()
	original := successors
	successors = table
	t.Cleanup(func() { successors = original })
}

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

func TestAWalkThatCannotArriveIsBounded(t *testing.T) {
	unhappy := []gen.RunStatus{gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut}
	swapSuccessors(t, map[gen.RunStatus][]gen.RunStatus{

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
