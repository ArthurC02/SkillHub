//go:build windows

package localdrv

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

const (
	errNoSuchProcess = syscall.Errno(87)

	residueExitWait = 5 * time.Second
)

func terminateResidue(pid int) error {
	p, err := os.FindProcess(pid)
	if errors.Is(err, errNoSuchProcess) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = p.Release() }()
	killErr := p.Kill()
	exited := make(chan struct{})
	go func() {
		_, _ = p.Wait()
		close(exited)
	}()
	select {
	case <-exited:
		return nil
	case <-time.After(residueExitWait):
		return fmt.Errorf("process %d did not exit after kill: %w", pid, errors.Join(killErr, os.ErrDeadlineExceeded))
	}
}
