//go:build windows

package localdrv

import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	jobMsgActiveProcessLimit = 3
	jobMsgJobMemoryLimit     = 10
)

type jobCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

type jobTree struct {
	mu   sync.Mutex
	job  windows.Handle
	port windows.Handle
	hits limitHits
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

	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("create completion port: %w", err)
	}
	closeBoth := func() {
		_ = windows.CloseHandle(job)
		_ = windows.CloseHandle(port)
	}
	assoc := jobCompletionPort{CompletionPort: port}
	if _, err := windows.SetInformationJobObject(
		job, windows.JobObjectAssociateCompletionPortInformation,
		uintptr(unsafe.Pointer(&assoc)), uint32(unsafe.Sizeof(assoc)),
	); err != nil {
		closeBoth()
		return fmt.Errorf("associate completion port: %w", err)
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
		closeBoth()
		return fmt.Errorf("set job limits: %w", err)
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		closeBoth()
		return fmt.Errorf("open process for job assignment: %w", err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		closeBoth()
		return fmt.Errorf("assign process to job: %w", err)
	}

	t.mu.Lock()
	t.job = job
	t.port = port
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
	job, port := t.job, t.port
	t.job, t.port = 0, 0
	t.mu.Unlock()
	if job == 0 {
		return nil
	}
	err := windows.CloseHandle(job)
	_ = windows.CloseHandle(port)
	return err
}

func (t *jobTree) limitHits() limitHits {
	t.mu.Lock()
	defer t.mu.Unlock()
	for t.port != 0 {
		var message uint32
		var key uintptr
		var overlapped *windows.Overlapped
		if windows.GetQueuedCompletionStatus(t.port, &message, &key, &overlapped, 0) != nil {
			break
		}
		switch message {
		case jobMsgActiveProcessLimit:
			t.hits.PidsLimitHit = true
		case jobMsgJobMemoryLimit:
			t.hits.MemoryLimitHit = true
		}
	}
	return t.hits
}

func resourceEnforcement() ResourceEnforcement {
	return ResourceEnforcement{Memory: true, Processes: true}
}

func reaping() Reaping { return Reaping{Descendants: true, Detached: true} }

func rootless() bool { return !windows.GetCurrentProcessToken().IsElevated() }
