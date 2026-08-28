package localdrv

// processTree is the platform-specific process-tree lifecycle: everything a
// bare os/exec.Cmd does not give you across the tree a workload spawns, not
// just the one process it starts directly. Windows uses a Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE; anything else uses a POSIX process group
// and kill(-pgid). Both are documented and measured to different degrees — see
// tree_windows.go and tree_unix.go.
//
// No third-party process-management dependency is used for either half
// (02:PORT-010's acceptance criterion, ADR-059 decision 4): only
// golang.org/x/sys/windows on Windows, already in this module's dependency
// tree, and the standard library's own os/exec and syscall everywhere else.
import "os/exec"

type processTree interface {
	// configure sets whatever SysProcAttr fields the platform needs before
	// cmd.Start(). On the POSIX side this puts the child in a new process
	// group at exec time; on Windows there is nothing to set here, because a
	// Job Object is a separate kernel object attached after the process
	// already exists (attach).
	configure(cmd *exec.Cmd)
	// attach binds the just-started process to this tree's reap boundary. lim
	// is applied only where a platform can actually enforce it — see
	// ResourceEnforcement in localdrv.go for which fields that is, on which
	// platform, and why the answer differs.
	attach(pid int, lim treeLimits) error
	// terminate kills the whole tree rooted at pid, not just pid itself.
	terminate(pid int) error
	// release lets go of whatever handle this tree holds. On Windows this is
	// itself what guarantees the kill (closing the last handle to a
	// KILL_ON_JOB_CLOSE job kills everything still in it), so it is called
	// unconditionally on Remove even when terminate already ran.
	release() error
}

// treeLimits is what a process tree can be asked to enforce at the OS level.
// A zero field means "do not attempt to enforce this one" — not "enforce a
// limit of zero".
type treeLimits struct {
	MemoryBytes  int64
	MaxProcesses int64
}

// Reaping is what this driver's Stop actually ends on this platform, as
// detected rather than as intended. It exists because the two platforms differ
// on precisely the case this driver was built for, and 2026-08-29's CI run is
// what forced it out of a comment and onto the type: tree_unix.go had written
// the setsid caveat down correctly, TestReapsWholeProcessTree asserted
// whole-tree reaping unconditionally anyway, and Linux CI failed. A limitation
// a test does not know about is a limitation nobody is holding.
//
// The asymmetry is not cosmetic. A Windows Job Object owns every descendant
// whether or not it wants to be owned; a POSIX process group is something a
// descendant leaves by calling setsid(). So the deliberately-detached
// process — the one an unmodified workload does not produce, and a hostile or
// careless one does — is reaped on Windows and is not reaped on Linux.
type Reaping struct {
	// Descendants: ordinary children and grandchildren, the ones that stayed
	// in the group or job they were started in. True on both platforms.
	Descendants bool
	// Detached: a descendant that deliberately left — setsid() on POSIX,
	// which Node spells `detached: true`. A Job Object still holds it; a
	// process group no longer contains it.
	Detached bool
}
