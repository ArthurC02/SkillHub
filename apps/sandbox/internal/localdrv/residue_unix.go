//go:build !windows

package localdrv

import "syscall"

func terminateResidue(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
