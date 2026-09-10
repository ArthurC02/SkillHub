//go:build windows

package localdrv

import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type jobTree struct {
	mu  sync.Mutex
	job windows.Handle
}

func newProcessTree() processTree { return &jobTree{} }

func (t *jobTree) configure(cmd *exec.Cmd) {}

// Assigning the process to a job with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// makes closing the job handle kill every process in it, descendants
// included, since Windows tracks job membership by process lineage.
func (t *jobTree) attach(pid int, lim treeLimits) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}

	flags := uint32(windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE)
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	if lim.MemoryBytes > 0 {
		flags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = uintptr(lim.MemoryBytes)
	}
	if lim.MaxProcesses > 0 {
		flags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		info.BasicLimitInformation.ActiveProcessLimit = uint32(lim.MaxProcesses)
	}
	info.BasicLimitInformation.LimitFlags = flags

	if _, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("set job limits: %w", err)
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("open process for job assignment: %w", err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("assign process to job: %w", err)
	}

	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
	return nil
}

func (t *jobTree) terminate(pid int) error {
	t.mu.Lock()
	job := t.job
	t.mu.Unlock()
	if job == 0 {
		return nil
	}
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return fmt.Errorf("terminate job object: %w", err)
	}
	return nil
}

func (t *jobTree) release() error {
	t.mu.Lock()
	job := t.job
	t.job = 0
	t.mu.Unlock()
	if job == 0 {
		return nil
	}
	return windows.CloseHandle(job)
}

func resourceEnforcement() ResourceEnforcement {
	return ResourceEnforcement{Memory: true, Processes: true}
}

func reaping() Reaping { return Reaping{Descendants: true, Detached: true} }

func rootless() bool { return !windows.GetCurrentProcessToken().IsElevated() }
