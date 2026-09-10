//go:build !windows

package localdrv

import (
	"os/exec"
	"strings"
	"testing"
)

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
