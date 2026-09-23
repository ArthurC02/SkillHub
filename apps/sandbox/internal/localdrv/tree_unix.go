//go:build !windows

package localdrv

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type pgroupTree struct {
	mu    sync.Mutex
	pid   int
	limit *cgroup
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

	enf := resourceEnforcement()
	if !enf.Memory {
		lim.MemoryBytes = 0
	}
	if !enf.Processes {
		lim.MaxProcesses = 0
	}
	if lim.MemoryBytes == 0 && lim.MaxProcesses == 0 {
		return nil
	}

	limit, err := newCgroup(fmt.Sprintf("run-%d", pid), lim)
	if err != nil {
		return fmt.Errorf("bind the workload to the ceilings this driver claims: %w", err)
	}
	if err := limit.add(pid); err != nil {
		_ = limit.remove()
		return fmt.Errorf("move the workload under its ceilings: %w", err)
	}
	t.mu.Lock()
	t.limit = limit
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

func (t *pgroupTree) release() error {
	t.mu.Lock()
	limit := t.limit
	t.limit = nil
	t.mu.Unlock()
	if limit == nil {
		return nil
	}
	return limit.remove()
}

func resourceEnforcement() ResourceEnforcement { return cgroupEnforcement() }

func reaping() Reaping { return Reaping{Descendants: true, Detached: false} }

func rootless() bool { return os.Geteuid() != 0 }
