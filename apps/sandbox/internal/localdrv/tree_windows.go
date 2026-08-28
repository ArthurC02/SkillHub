//go:build windows

package localdrv

// Windows process-tree reaping via a Job Object.
//
// Measured in docs/plans/mvp/m6/report-local-driver.md §2, on this same
// platform: a probe that spawned a child which re-spawned its own grandchild,
// then compared two ways of ending the tree. Killing only the direct child
// left the grandchild running (leaked=1). Assigning the child to a Job Object
// created with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and then closing the job
// handle did not (leaked=0) — Windows walks process creation lineage for job
// membership, so a grandchild spawned by a job member is itself a member
// unless it explicitly opts out, which nothing in this driver's workload does.
//
// This needs no administrator privilege: report §2 ran as an ordinary user.
// It needs no third-party dependency either (ADR-059 decision 4): every symbol
// below is already in golang.org/x/sys/windows, itself already indirect in
// this module before this package existed.
import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobTree holds one Job Object handle. A zero job means none has been created
// yet (CreateJobObject never returns a valid handle equal to 0 on success), so
// it doubles as the "nothing to release" sentinel.
type jobTree struct {
	mu  sync.Mutex
	job windows.Handle
}

func newProcessTree() processTree { return &jobTree{} }

// configure sets nothing: a Job Object is a separate kernel object, created
// and attached after the process already exists (attach), not a SysProcAttr
// flag set before Start.
func (t *jobTree) configure(cmd *exec.Cmd) {}

// attach creates a Job Object, sets its limits, and assigns the process to it.
//
// There is a window this cannot close: CreateProcess and
// AssignProcessToJobObject are two separate calls, and Go's os/exec returns
// from Start() with the child already resumed — CREATE_SUSPENDED and
// PROC_THREAD_ATTRIBUTE_JOB_LIST are both unavailable from os/exec
// (golang/go#32404, golang/go#44005, both still open per report §4). A child
// that spawns a grandchild in the few microseconds before AssignProcessToJobObject
// runs could in principle produce a grandchild the job never claims. Buildkite's
// own process package (the ~120-line reference report §3 points at) accepts
// this same gap rather than working around it, and so does this package.
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
		windows.CloseHandle(job)
		return fmt.Errorf("set job limits: %w", err)
	}

	// PROCESS_SET_QUOTA is what AssignProcessToJobObject itself requires;
	// PROCESS_TERMINATE is what this driver's own terminate() needs later.
	// Report §2 lists both as the exact rights a job assignment needs — no
	// broader access right is requested.
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("open process for job assignment: %w", err)
	}
	defer windows.CloseHandle(proc)

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("assign process to job: %w", err)
	}

	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
	return nil
}

// terminate kills every process the job holds, immediately. This is the
// explicit form of the same guarantee release() gets for free from
// KILL_ON_JOB_CLOSE — both are called on the way out (Remove), because a
// caller may reach Remove without ever calling Stop first.
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

// release closes the job handle. Closing the last handle to a job created with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE kills everything still in it — this is not
// a courtesy close, it is the second, unconditional half of the reaping
// guarantee this package makes (package doc point 1: it is also why a service
// restart kills every in-flight run, which is why Adopt() has nothing to find).
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

// resourceEnforcement: Job Objects cap memory (JOB_OBJECT_LIMIT_JOB_MEMORY) and
// process count (JOB_OBJECT_LIMIT_ACTIVE_PROCESS) without needing an
// administrator (report §2 ran as an ordinary user). CPU is left unenforced on
// purpose: JOBOBJECT_CPU_RATE_CONTROL_INFORMATION is a second, separate
// SetInformationJobObject call this package does not make, so it must not be
// claimed (report §3 calls the missing struct "自己宣告約 5 行" — cheap to
// add, but nothing here has measured what it actually does to node under load,
// and an unmeasured claim is exactly what this driver exists to not make).
func resourceEnforcement() ResourceEnforcement {
	return ResourceEnforcement{Memory: true, Processes: true}
}

// reaping: a Job Object created without JOB_OBJECT_LIMIT_BREAKAWAY_OK holds
// every process assigned to it and every process those go on to create, and it
// holds them whether or not they asked to be held. There is no Windows
// equivalent of setsid() that walks a process out of a job it was assigned to,
// so both answers here are yes — and the detached one is the answer this
// package exists for (see the fixture note in testdata/reaper.mjs for why the
// ordinary case is already covered by Node itself on this platform).
func reaping() Reaping { return Reaping{Descendants: true, Detached: true} }
