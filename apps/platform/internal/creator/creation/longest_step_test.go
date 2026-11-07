package creation

import (
	"testing"
	"time"
)

func TestTheLongestStepCoversTheLongestCallALimitMayAllow(t *testing.T) {
	at := Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 1, MaxToolCalls: 1,
		CallTimeout: 120 * time.Second, SessionTimeout: time.Hour, Retention: 3 * time.Hour, MaxOutputTokens: 500}
	if !at.Valid() {
		t.Fatal("a 120s call timeout is refused; the boundary under test moved")
	}
	past := at
	past.CallTimeout = 121 * time.Second
	if past.Valid() {
		t.Fatal("a 121s call timeout is accepted, so a step may outlast LongestStep")
	}
	if worst := at.activeStep() + attemptWrapUp; LongestStep < worst {
		t.Errorf("LongestStep = %s, shorter than the longest allowed step %s", LongestStep, worst)
	}
}
