//go:build !windows

package localdrv

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRootlessMatchesTheIdCommand checks the privilege detection against a
// second, independent source rather than restating os.Geteuid(): `id -u`
// prints the effective uid the kernel would use for a permission check, and 0
// is root.
//
// isolation.rootless is a dispatch gate, so a detection that quietly inverts
// puts a node into rotation rather than taking it out — which is why this
// asserts against something outside the package instead of against the same
// syscall spelled twice.
func TestRootlessMatchesTheIdCommand(t *testing.T) {
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		t.Skipf("id(1) unavailable: %v", err)
	}
	rootPerID := strings.TrimSpace(string(out)) == "0"
	if got := rootless(); got == rootPerID {
		t.Fatalf("rootless() = %v while `id -u` says root=%v; the two must be opposites", got, rootPerID)
	}
}
