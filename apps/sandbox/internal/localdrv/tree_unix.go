//go:build !windows

package localdrv

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type pgroupTree struct {
	mu  sync.Mutex
	pid int
}

func newProcessTree() processTree { return &pgroupTree{} }

func (t *pgroupTree) configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func (t *pgroupTree) attach(pid int, lim treeLimits) error {
	t.mu.Lock()
	t.pid = pid
	t.mu.Unlock()
	return nil
}

// A negative pid signals the whole process group rather than one process.
func (t *pgroupTree) terminate(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func (t *pgroupTree) release() error { return nil }

func resourceEnforcement() ResourceEnforcement {
	return ResourceEnforcement{}
}

func reaping() Reaping { return Reaping{Descendants: true, Detached: false} }

func rootless() bool { return os.Geteuid() != 0 }
