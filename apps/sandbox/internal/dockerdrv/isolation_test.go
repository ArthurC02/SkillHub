package dockerdrv

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestOnlyAUserSpaceKernelCountsAsStrongIsolation(t *testing.T) {
	for _, tc := range []struct {
		runtime string
		want    sandbox.IsolationStrength
	}{
		{"runsc", sandbox.IsolationStrong},
		{"runc", sandbox.IsolationWeak},
		{"", sandbox.IsolationWeak},
	} {
		t.Run(tc.runtime, func(t *testing.T) {
			d := &Driver{cfg: Config{Runtime: tc.runtime}}
			if got := d.Isolation(); got != tc.want {
				t.Errorf("Isolation() with runtime %q = %q, want %q", tc.runtime, got, tc.want)
			}
		})
	}
}
