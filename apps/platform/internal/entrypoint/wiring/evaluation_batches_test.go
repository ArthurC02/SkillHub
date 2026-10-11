package wiring

import (
	"errors"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func TestABatchWaitsOnlyForRefusalsThatTimeFixes(t *testing.T) {
	other := errors.New("connection reset")
	if err := batchRunError(nil); err != nil {
		t.Errorf("a started run: got %v, want nil", err)
	}
	if err := batchRunError(run.ErrDispatchHalted); !errors.Is(err, eval.ErrRunMustWait) {
		t.Errorf("a halted dispatch: got %v, want the batch to wait", err)
	}
	for _, gone := range []error{run.ErrNotFound, run.ErrPreflightTargetNotFound} {
		if refused, ok := errors.AsType[eval.RunRefusedError](batchRunError(gone)); !ok || refused.Reason != "target_gone" {
			t.Errorf("%v: got %v, want the batch stopped as target_gone", gone, refused)
		}
	}
	if err := batchRunError(other); !errors.Is(err, other) || errors.Is(err, eval.ErrRunMustWait) {
		t.Errorf("an infrastructure error: got %v, want it returned unchanged for a retry", err)
	}
}
