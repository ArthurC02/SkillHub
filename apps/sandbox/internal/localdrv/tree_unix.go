//go:build !windows

package localdrv

// POSIX process-tree reaping via a process group.
//
// UNTESTED. Everything in this file was written from report §3's reading of
// go-cmd/cmd's own cmd_linux.go (Setpgid at exec time, kill(-pgid) to end the
// group) — a shape the report found correct there, in contrast to that same
// project's Windows half, which is a documented no-op (see tree_windows.go's
// doc comment for what that gap looks like measured). Nobody has run this file
// against a real grandchild on Linux (report §5), because the only machine
// available while building this driver was Windows (report §1). Whoever wires
// this driver up for a Linux deployment should not treat this file as measured
// until they have run this package's TestReapsWholeProcessTree here and
// watched it pass.
//
// One known gap even if it is correct: a grandchild that calls setsid() (a
// deliberately detached daemon, not an ordinary forked or spawned child) leaves
// the process group entirely and this file's kill(-pgid) will not reach it.
// That is a real limitation of process-group-based reaping in general, not
// specific to this implementation — and it is a different failure mode from
// the Windows one this package was built to avoid, where an *ordinary*
// grandchild survives a plain kill of just the parent.
import (
	"os/exec"
	"sync"
	"syscall"
)

type pgroupTree struct {
	mu  sync.Mutex
	pid int
}

func newProcessTree() processTree { return &pgroupTree{} }

// configure puts the child in a new process group of its own at exec time, so
// every process it goes on to fork or spawn inherits that same group unless it
// deliberately leaves it (see the setsid caveat above).
func (t *pgroupTree) configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// attach records the pid for terminate. lim is intentionally unused: without
// root, Go's SysProcAttr carries no RLimit field, and there is no verified
// rootless cgroup-delegation path this package attempts — see
// resourceEnforcement below and the package doc's point 2. Claiming
// enforcement this driver has not measured is exactly the failure mode report
// §0 documents for go-cmd/cmd's Windows half: a claim the code behind it did
// not back up.
func (t *pgroupTree) attach(pid int, lim treeLimits) error {
	t.mu.Lock()
	t.pid = pid
	t.mu.Unlock()
	return nil
}

// terminate signals the whole process group. A negative pid targets the group
// rather than the single process (man 2 kill); ESRCH means the group is
// already gone, which is the state this call was asked to reach, not a
// failure to report.
func (t *pgroupTree) terminate(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

// release: there is no handle to hold open here, unlike the Windows job — a
// process group is not a kernel object with a lifetime of its own to close.
func (t *pgroupTree) release() error { return nil }

// resourceEnforcement: nothing here is OS-enforced, on either count. This is a
// deliberately conservative "no" rather than an attempt at the rootless cgroup
// path (UseCgroupFD, Go 1.20+) that Linux would need to give an honest "yes" —
// that path depends on the host's systemd having delegated the memory
// controller, which this package has no way to check and does not assume
// (package doc point 2; report §4①).
func resourceEnforcement() ResourceEnforcement {
	return ResourceEnforcement{}
}

// reaping: kill(-pgid) reaches everything still in the group, which is every
// ordinary descendant — and nothing that called setsid(), because setsid()
// creates a new session and a new process group, and the pid this driver
// signals is no longer that process's group leader.
//
// This is a limitation of process-group reaping in general, not of this
// implementation, and it is deliberately reported rather than worked around.
// The unprivileged ways out are real but each costs more than this driver's
// purpose is worth: PR_SET_CHILD_SUBREAPER plus a /proc descendant sweep sets
// process-wide state on a binary this package does not own, and a PID
// namespace via user namespaces changes what the workload sees and depends on
// unprivileged userns being enabled at all. M6's target machine is Windows
// (m6/README), production Linux isolates with gVisor through dockerdrv, and
// this driver never carries untrusted content on either — so the honest "no"
// is the answer that costs least and hides nothing.
func reaping() Reaping { return Reaping{Descendants: true, Detached: false} }
