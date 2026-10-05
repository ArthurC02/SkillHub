package dockerdrv

import "testing"

func TestHostPidsLimitConvertsOnlyUnderTheUserSpaceKernel(t *testing.T) {
	cases := []struct {
		name    string
		runtime string
		maxPIDs int64
		want    int64
	}{
		{"runsc at the lower bound", "runsc", 1, 66},
		{"runsc at the default", "runsc", 256, 576},
		{"runc passes through at the lower bound", "runc", 1, 1},
		{"runc passes through at the default", "runc", 256, 256},
		{"empty runtime passes through at the lower bound", "", 1, 1},
		{"empty runtime passes through at the default", "", 256, 256},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hostPidsLimit(c.runtime, c.maxPIDs); got != c.want {
				t.Errorf("hostPidsLimit(%q, %d) = %d, want %d", c.runtime, c.maxPIDs, got, c.want)
			}
		})
	}
}
