package wiring

import (
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/batch"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestABatchWaitsOnlyForRefusalsThatTimeFixes(t *testing.T) {
	other := errors.New("connection reset")
	if err := batchRunError(nil); err != nil {
		t.Errorf("a started run: got %v, want nil", err)
	}
	if err := batchRunError(run.ErrDispatchHalted); !errors.Is(err, batch.ErrRunMustWait) {
		t.Errorf("a halted dispatch: got %v, want the batch to wait", err)
	}
	for _, gone := range []error{run.ErrNotFound, run.ErrPreflightTargetNotFound} {
		if err := batchRunError(gone); !errors.Is(err, batch.ErrTargetGone) || !errors.Is(err, gone) {
			t.Errorf("%v: got %v, want the batch told its target is gone", gone, err)
		}
		if err := batchAnswerError(gone); !errors.Is(err, batch.ErrNoFurtherRound) {
			t.Errorf("%v while answering: got %v, want no further round", gone, err)
		}
	}
	if err := batchRunError(other); !errors.Is(err, other) || errors.Is(err, batch.ErrRunMustWait) {
		t.Errorf("an infrastructure error: got %v, want it returned unchanged for a retry", err)
	}
}
