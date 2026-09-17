package localdrv

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestARunOnTheHostDeclaresNoIsolation(t *testing.T) {
	if got := (&Driver{}).Isolation(); got != sandbox.IsolationNone {
		t.Errorf("Isolation() = %q, want %q: this driver runs the workload as a host process",
			got, sandbox.IsolationNone)
	}
}
