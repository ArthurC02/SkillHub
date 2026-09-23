//go:build windows

package localdrv

import "os"

func terminateResidue(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
