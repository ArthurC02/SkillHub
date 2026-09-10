//go:build windows

package localdrv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootlessMatchesTheTokenWhoamiReports(t *testing.T) {

	whoami := filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe")
	out, err := exec.Command(whoami, "/groups").Output()
	if err != nil {
		t.Skipf("%s unavailable: %v", whoami, err)
	}
	// S-1-16-12288 is the well-known SID for the High mandatory integrity level.
	elevatedPerWhoami := strings.Contains(string(out), "S-1-16-12288")
	if got := rootless(); got == elevatedPerWhoami {
		t.Fatalf("rootless() = %v while this process's token integrity level says elevated=%v; "+
			"the two must be opposites", got, elevatedPerWhoami)
	}
}
