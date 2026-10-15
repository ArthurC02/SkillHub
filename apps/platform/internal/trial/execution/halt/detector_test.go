package halt

import (
	"testing"
	"time"
)

func TestNodeAndPoolThresholds(t *testing.T) {
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{
		{"closed beta node, 2 slots at 50%", 2, 1},
		{"4 slots at 50%", 4, 2},
		{"early growth node, 8 slots at 50%", 8, 4},
		{"an undeclared or unreachable node falls back to the floor", 0, 1},

		{"3 slots at 50% rounds up, and the floor of 1 cannot hide it", 3, 2},
	} {
		if got := haltThreshold(tc.slots, 2, 1); got != tc.want {
			t.Errorf("node threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{

		{"closed beta pool, 4 slots at 25% under the floor", 4, 2},
		{"early growth pool, 20 slots at 25%", 20, 5},
		{"6 slots at 25% rounds up to a whole resource", 6, 2},
		{"no slots declared anywhere", 0, 2},
	} {
		if got := haltThreshold(tc.slots, 4, 2); got != tc.want {
			t.Errorf("pool threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
}

func TestAnOrphanCountAtTheThresholdBreachesIt(t *testing.T) {
	for _, tc := range []struct {
		persistent, threshold int64
		want                  bool
	}{{1, 2, false}, {2, 2, true}, {3, 2, true}} {
		if got := (orphanCount{persistent: tc.persistent, threshold: tc.threshold}).breached(); got != tc.want {
			t.Errorf("%d persistent against %d: breached = %v, want %v", tc.persistent, tc.threshold, got, tc.want)
		}
	}
}

func TestMaskingActivityRefusesWithoutTraceService(t *testing.T) {
	if _, err := (Detector{}).maskingActivity(t.Context(), time.Now(), time.Now()); err == nil {
		t.Error("masking activity succeeded without Trace owner service")
	}
}
