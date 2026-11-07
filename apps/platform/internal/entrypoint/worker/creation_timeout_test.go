package worker

import (
	"testing"
	"time"
)

func TestTheCreationStepJobOutlivesTheLongestStepALimitMayAllow(t *testing.T) {
	const longestAllowedStep = 120*time.Second + 10*time.Second + 60*time.Second
	if got := (&CreationStepWorker{}).Timeout(nil); got <= longestAllowedStep {
		t.Errorf("job timeout = %s, want more than the %s a step may take", got, longestAllowedStep)
	}
}
