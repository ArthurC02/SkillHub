package creation

import (
	"testing"
	"time"
)

func TestASessionIsKeptUntilAStepStartedAtItsDeadlineHasSettled(t *testing.T) {
	limits := func(retention time.Duration) Limits {
		return Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 1, MaxToolCalls: 1,
			CallTimeout: time.Second, SessionTimeout: time.Minute, Retention: retention, MaxOutputTokens: 1}
	}
	for _, c := range []struct {
		name      string
		retention time.Duration
		valid     bool
	}{
		{"retention equal to the session timeout", time.Minute, false},
		{"one second short of the last step settling", time.Minute + 70*time.Second, false},
		{"exactly long enough for the last step to settle", time.Minute + 71*time.Second, true},
	} {
		if got := limits(c.retention).Valid(); got != c.valid {
			t.Errorf("%s: Valid() = %v, want %v", c.name, got, c.valid)
		}
	}
}
