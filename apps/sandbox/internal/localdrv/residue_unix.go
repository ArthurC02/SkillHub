//go:build !windows

package localdrv

import "syscall"

// A negative pid signals the whole process group rather than one process.
func terminateResidue(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
