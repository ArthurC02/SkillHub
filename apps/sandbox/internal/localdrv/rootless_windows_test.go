//go:build windows

package localdrv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRootlessMatchesTheTokenWhoamiReports checks the elevation detection
// against a second, independent source rather than restating the same call:
// `whoami /groups` prints the token's own mandatory integrity level, and
// S-1-16-12288 (High) is what an elevated process carries. A non-elevated
// process gets S-1-16-8192 (Medium).
//
// The point is not that the two agree today, it is that a change to how
// rootless() asks the question has to keep agreeing with what the OS itself
// says about this process. isolation.rootless is a dispatch gate, so a
// detection that quietly inverts takes a node into rotation, not out of it.
func TestRootlessMatchesTheTokenWhoamiReports(t *testing.T) {
	// The absolute path, not PATH: a developer shell (Git Bash, MSYS) puts its
	// own POSIX whoami in front, and that one has no /groups to print.
	whoami := filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe")
	out, err := exec.Command(whoami, "/groups").Output()
	if err != nil {
		t.Skipf("%s unavailable: %v", whoami, err)
	}
	elevatedPerWhoami := strings.Contains(string(out), "S-1-16-12288")
	if got := rootless(); got == elevatedPerWhoami {
		t.Fatalf("rootless() = %v while this process's token integrity level says elevated=%v; "+
			"the two must be opposites", got, elevatedPerWhoami)
	}
}
