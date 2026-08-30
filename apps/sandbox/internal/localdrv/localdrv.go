// Package localdrv is the clean test mode's execution Driver (02:PORT-010): one
// host process per attempt instead of one container per attempt. It spawns the
// identical workload dockerdrv does — a single `node run.mjs` — with /work and
// /out replaced by a per-run directory on the host, because there is no
// isolation boundary left to put a container image inside: the whitelist that
// gates container execution rejects any PE the caller has to bring with it
// (ADR-059 background).
//
// `clean` is not a weaker sandbox, it is no sandbox at all. What stops this
// driver from ever being handed untrusted content is the platform's content
// gate — trial/execution/schedule.go's requireCuratedContent, called by
// dispatch() before a provider is even chosen, which passes only material in a
// catalogue workspace or curated at exactly the version being run — not
// anything in this package.
//
// Until 2026-08-30 this sentence named SKILLHUB_CLEAN_MODE and the isolation
// whitelist (ADR-059 decision 3) instead, and that was wrong in a way worth
// recording: the whitelist decides which DRIVER may be dispatched to and has no
// branch that can see a skill, a version or a workspace. Three documents each
// named one of the others as the enforcement point and none of them was one
// (02:PORT-010, 04 丙-85). What actually held was that an operator running this
// on their own machine would not fork a stranger's skill and press run — a
// habit, not a gate. The gate exists now; this comment points at it rather than
// at a neighbour.
//
// What this package is honest about is narrower and is a
// lifecycle guarantee, not a containment one: spawn, wait, collect, and reap
// the *entire* process tree when told to stop — which a bare
// os/exec.Cmd.Process.Kill() does not do. Measured in
// docs/plans/mvp/m6/report-local-driver.md §2: killing only the parent left a
// spawned grandchild alive on Windows (leaked=1); closing the Job Object
// handle this package uses did not (leaked=0).
//
// Three things this driver cannot do are ADR-059 decision 5, and are written
// down here rather than left to a comment nobody reads while wiring it up:
//
//  1. Adopt() returns nothing. A restarted process holds no local record to
//     rebuild a workload from, and on Windows JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
//     means the Job handle's death already killed anything still running under
//     it — there would be nothing left to adopt even if the record survived.
//  2. Resource ceilings are enforced only where they are actually enforced.
//     Windows Job Objects cap memory and process count without needing an
//     administrator. Linux without root has no equivalent this package
//     attempts: Go's SysProcAttr carries no RLimit field, and the rootless
//     cgroup-delegation path (UseCgroupFD, Go 1.20+) depends on the host's
//     systemd having delegated the memory controller, which this package has
//     no way to verify was done and does not assume. See ResourceEnforcement.
//     The one lever available on every platform is passed to node itself
//     (--max-old-space-size): a request to the V8 heap, not an OS-enforced
//     wall, and every host process still shares whatever native memory the
//     OS gives it.
//  3. Stop's grace period is not a negotiation on either platform. run.mjs
//     installs no signal handler (`grep process.on` is zero hits in it), so
//     there is nobody on the other end to ask nicely. The grace window still
//     matters — it is PDM-005's cooperative collection window, the time
//     between the workload announcing WorkloadDone and this driver reading
//     the trace and artifacts back out before the hard kill — it is just not
//     a request the workload can decline or shorten.
package localdrv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// logTailBytes bounds how much of the workload's own stdout/stderr rides back
// in Outcome.Output. Matches dockerdrv's own bound (TRACE-004 owns the rest).
const logTailBytes = 32 << 10

// Config is deployment policy for the driver. The shape mirrors dockerdrv.Config
// deliberately: a pinned runtime image there is a pinned script path here.
type Config struct {
	// NodeBin is the node executable used to run the workload. Empty resolves
	// "node" via exec.LookPath at New time, so a missing runtime is a
	// construction-time error rather than a surprise on the first Start.
	NodeBin string
	// RunnerScript is the absolute host path to run.mjs (or a build of it).
	// This driver does not vendor or ship it — same responsibility split as
	// dockerdrv.Config.Image, just a filesystem path instead of a registry
	// reference the deployment pins.
	RunnerScript string
	// BaseDir is the root this driver creates one directory per attempt
	// under: <BaseDir>/<provider_run_id>/{work,out}. Defaults to
	// filepath.Join(os.TempDir(), "skillhub-clean") when empty.
	BaseDir string
}

// run is what this driver keeps about one attempt between Start and Remove.
// Unlike dockerdrv, which keeps nothing beyond a container name and reads
// everything else back from the daemon, a host process has no daemon to ask —
// this struct is the only place any of it lives.
type run struct {
	cmd     *exec.Cmd
	tree    processTree
	tail    *tailWriter
	workDir string
	outDir  string

	done    chan struct{} // closed once cmd.Wait() has returned
	outcome sandbox.Outcome
	waitErr error
}

// Driver implements sandbox.Driver by spawning one host process per attempt.
type Driver struct {
	cfg Config

	mu   sync.Mutex
	runs map[string]*run
}

// Compile-time proof this is a drop-in second Driver: 02:PORT-010 forbids a
// second dispatch path, and Manager only ever calls through this interface.
var _ sandbox.Driver = (*Driver)(nil)

// New validates the deployment policy and resolves the node runtime once, so a
// missing or misconfigured runtime fails at construction rather than on the
// first dispatched run.
func New(cfg Config) (*Driver, error) {
	if cfg.NodeBin == "" {
		cfg.NodeBin = "node"
	}
	if _, err := exec.LookPath(cfg.NodeBin); err != nil {
		return nil, fmt.Errorf("node runtime not found (%s): %w", cfg.NodeBin, err)
	}
	if cfg.RunnerScript == "" {
		return nil, errors.New("a runner script path is required (run.mjs)")
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = filepath.Join(os.TempDir(), "skillhub-clean")
	}
	return &Driver{cfg: cfg, runs: map[string]*run{}}, nil
}

// Close releases whatever this driver still holds when the process using it is
// shutting down. There is no persistent daemon connection to close here (unlike
// dockerdrv.Close) — this just makes sure a shutdown does not leave stray host
// processes behind if some caller skipped Remove.
func (d *Driver) Close() error {
	d.mu.Lock()
	ids := make([]string, 0, len(d.runs))
	for id := range d.runs {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	for _, id := range ids {
		_ = d.Remove(context.Background(), id)
	}
	return nil
}

// paths is the per-attempt directory layout: work and out live under one
// directory per id, so Remove can delete both with a single RemoveAll.
func (d *Driver) paths(id string) (workDir, outDir string) {
	root := filepath.Join(d.cfg.BaseDir, id)
	return filepath.Join(root, "work"), filepath.Join(root, "out")
}

func (d *Driver) get(id string) *run {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs[id]
}

// take removes and returns the entry, so Remove is safe to call twice: the
// second call finds nothing and succeeds anyway (SBX-009, iron rule 9).
func (d *Driver) take(id string) *run {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.runs[id]
	delete(d.runs, id)
	return r
}

// Start creates the per-run directories, spawns the workload bound to this
// platform's reap boundary, and pushes the run's inputs into it. It returns
// before the workload finishes; Wait follows it to the end.
func (d *Driver) Start(ctx context.Context, id string, req sandbox.RunRequest) error {
	workDir, outDir := d.paths(id)
	dirs := []string{
		workDir, outDir,
		filepath.Join(outDir, traceSubdir),
		filepath.Join(outDir, artifactSubdir),
		inputDir(workDir),
		datasetDir(workDir),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("prepare run directories: %w", err)
		}
	}

	cmd := exec.Command(d.cfg.NodeBin, d.cfg.RunnerScript)
	cmd.Dir = workDir
	cmd.Env = env(req, workDir, outDir)

	tail := &tailWriter{limit: logTailBytes}
	cmd.Stdout = tail
	cmd.Stderr = tail

	tree := newProcessTree()
	tree.configure(cmd)

	r := &run{cmd: cmd, tree: tree, tail: tail, workDir: workDir, outDir: outDir, done: make(chan struct{})}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start workload: %w", err)
	}

	lim := treeLimits{MemoryBytes: req.ResourceLimits.MemoryBytes, MaxProcesses: req.ResourceLimits.MaxPIDs}
	if err := tree.attach(cmd.Process.Pid, lim); err != nil {
		// The process exists but is not bound to a reap boundary: kill it
		// outright rather than leave a sandbox this driver could not track.
		_ = cmd.Process.Kill()
		_ = tree.release()
		return fmt.Errorf("bind workload to its reap boundary: %w", err)
	}

	d.mu.Lock()
	d.runs[id] = r
	d.mu.Unlock()

	go d.reap(r)

	// SBX-008's shape without the exec boundary: inputs go in after the
	// process exists, and run.mjs waits for the ready marker before it
	// touches any of them. Unlike dockerdrv's pushInputs, there is no
	// "container might already be gone" race to classify here — this writes
	// straight to files this process itself just created.
	if err := d.pushInputs(ctx, r, req); err != nil {
		return err
	}
	return nil
}

// reap waits for the workload to exit and records its outcome. A non-zero exit
// is not a driver error (Manager reads that from Outcome.ExitCode); only a
// failure to wait on the process at all is.
func (d *Driver) reap(r *run) {
	waitErr := r.cmd.Wait()
	outcome := sandbox.Outcome{Output: r.tail.String()}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		outcome.ExitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		r.waitErr = waitErr
	}
	r.outcome = outcome
	close(r.done)
}

// Wait blocks until the workload exits or ctx is cancelled.
func (d *Driver) Wait(ctx context.Context, id string) (sandbox.Outcome, error) {
	r := d.get(id)
	if r == nil {
		return sandbox.Outcome{}, fmt.Errorf("no such sandbox: %s", id)
	}
	select {
	case <-r.done:
		return r.outcome, r.waitErr
	case <-ctx.Done():
		return sandbox.Outcome{}, ctx.Err()
	}
}

// Stop kills the workload's whole process tree after grace. There is no
// cooperative signal to send first (package doc, point 3): grace is spent
// waiting for the workload to finish on its own — most usefully, to run the
// collection handshake — not on a request it could act on.
func (d *Driver) Stop(ctx context.Context, id string, grace time.Duration) error {
	r := d.get(id)
	if r == nil {
		return nil // gone already; Stop has no not-found case either
	}
	if grace > 0 {
		select {
		case <-r.done:
			return nil
		case <-time.After(grace):
		case <-ctx.Done():
		}
	}
	select {
	case <-r.done:
		return nil
	default:
	}
	return r.tree.terminate(r.cmd.Process.Pid)
}

// Remove releases everything the handle holds and is idempotent: removing a
// handle nothing is held for is success (SBX-009, iron rule 9).
func (d *Driver) Remove(ctx context.Context, id string) error {
	r := d.take(id)
	if r == nil {
		return nil
	}
	_ = r.tree.terminate(r.cmd.Process.Pid)
	_ = r.tree.release()
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		// The reap goroutine did not observe the exit in time. Proceeding to
		// remove the directories anyway matches dockerdrv's own Force
		// removal: a slow-to-reap process must not hold a run's cleanup
		// hostage, and terminate+release above have already done everything
		// this driver can do to end it.
	}
	_ = os.RemoveAll(filepath.Dir(r.workDir)) // <BaseDir>/<id>, work and out together
	return nil
}

// Adopt reports nothing this driver still holds. ADR-059 decision 5①: a
// restarted process has no local record to rebuild a run from, and on Windows
// the Job handle's death (implicit in the process exiting) already took
// anything still running with it via KILL_ON_JOB_CLOSE — there is nothing left
// standing to adopt even in principle.
func (d *Driver) Adopt(ctx context.Context) ([]sandbox.Adopted, error) {
	return nil, nil
}

// Healthy reports whether the node runtime this driver was configured with can
// still be found and actually runs.
func (d *Driver) Healthy(ctx context.Context) bool {
	if _, err := exec.LookPath(d.cfg.NodeBin); err != nil {
		return false
	}
	return exec.CommandContext(ctx, d.cfg.NodeBin, "--version").Run() == nil
}

// ResourceEnforcement reports which of a run's resource ceilings this driver
// can actually make the OS hold to, on this platform, right now — not which
// ones a deployment wishes it could. Whatever wires this driver's Config into
// a sandbox.Config.MaxResources must not declare a ceiling stronger than what
// this returns: 02:PORT-010's acceptance criterion is that the capability
// declaration reflects what was detected, not what was intended.
//
// Every field here is one ResourceLimits ceiling, and cmd/sandboxd's
// unenforcedCeilings turns "no field claims it" into the name the capability
// declares as unheld. So a ceiling this struct does not mention is reported as
// unenforced, which is the direction that makes an omission loud: the three
// added on 2026-08-29 (CPU, Disk, OpenFiles) had been unenforced on both
// platforms since this package existed, and were declared as walls because
// nothing here said otherwise.
type ResourceEnforcement struct {
	Memory    bool // an OS-enforced memory ceiling
	Processes bool // an OS-enforced process-count ceiling
	// CPU: false on both platforms. Windows would need a second, separate
	// SetInformationJobObject call with JOBOBJECT_CPU_RATE_CONTROL_INFORMATION
	// (tree_windows.go says so and this is where that sentence becomes
	// machine-readable); POSIX has no unprivileged equivalent this package
	// attempts.
	CPU bool
	// Disk: false on both platforms. This driver's per-run directory is an
	// os.MkdirAll on the host filesystem with no quota behind it, where
	// dockerdrv gets a real wall from a tmpfs `size=`.
	Disk bool
	// OpenFiles: false on both platforms. dockerdrv passes a `nofile` ulimit to
	// the daemon; nothing here sets an rlimit on the child.
	OpenFiles bool
}

// ResourceEnforcement is answered per-platform in tree_windows.go / tree_unix.go
// (resourceEnforcement()): Windows Job Objects give a real yes to both without
// needing an administrator; this package does not attempt the Linux rootless
// cgroup path at all, for the reason in the package doc, so Linux answers no to
// both rather than a guess.
func (d *Driver) ResourceEnforcement() ResourceEnforcement {
	return resourceEnforcement()
}

// Reaping reports what Stop actually ends on this platform. Like
// ResourceEnforcement it is a detection, not an intention, and it is on the
// type rather than in a comment for the reason ADR-059 decision 5 gives: a
// thing this driver cannot do belongs somewhere a caller and a test can both
// read it. See the Reaping type in tree.go for why the two platforms differ.
func (d *Driver) Reaping() Reaping {
	return reaping()
}

// tailWriter keeps only the last limit bytes written to it, matching
// dockerdrv's own bound on how much of a workload's stdout/stderr rides back in
// the result (TRACE-004 is where the rest belongs). Docker can ask its daemon
// for a bounded log tail after the fact; a bare host process has no daemon to
// ask, so this captures the tail as the bytes stream past instead.
type tailWriter struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.limit {
		w.buf = w.buf[len(w.buf)-w.limit:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(string(w.buf))
}

// env builds the workload's environment. The variable names and the values
// they carry are the same contract dockerdrv's env() writes into a container's
// Config.Env — run.mjs reads paths and secrets from the environment, never from
// argv, so nothing about it needs to change for a host process instead of a
// container (ADR-059 §"背景"). It is duplicated here rather than imported: the
// two are unexported functions in different packages, and ADR-059's own
// "待決策" §1 leaves open whether this contract should move to a shared
// package — until it does, dockerdrv/docker.go's env() is the reference this
// one must be kept in step with by hand.
//
// Like dockerdrv, this builds the environment rather than inheriting one. It
// used to start from os.Environ() and override only its own keys, on the
// reasoning that a driver with no isolation gains none by trimming a variable
// list. That reasoning was wrong about what it was trimming: sandboxd is
// started by tools/cleanmode/start.mjs, which hands it DATABASE_URL (the
// control plane's own connection string) and SKILLHUB_SANDBOX_TOKEN (the
// provider port's bearer credential) — so os.Environ() was not "the host's
// ambient environment", it was a set of keys to the control plane, handed to
// an unsandboxed workload that has `env` in its allowed tools. ADR-059 signed
// off on "clean has no boundary"; it did not sign off on handing the workload
// the credentials, and iron rule 11 forbids them reaching a trace at all.
//
// So the host side is an allowlist: exactly what node needs to start on either
// platform, and nothing else. A denylist was the other option and is the wrong
// one for the same reason ADR-059 decision 3's own incident was a denylist —
// its next hole is the next variable somebody adds. Matching is
// case-insensitive because Windows environment names are (`Path`, not `PATH`).
func env(req sandbox.RunRequest, workDir, outDir string) []string {
	type kv struct{ k, v string }
	pairs := []kv{
		{"SKILLHUB_RUN_ID", req.RunID},
		{"SKILLHUB_RUN_ATTEMPT_ID", req.RunAttemptID},
		{"SKILLHUB_ATTEMPT", strconv.Itoa(req.Attempt)},
		{"SKILLHUB_WORKSPACE_ID", req.WorkspaceID},
		{"SKILLHUB_WORKDIR", workDir},
		{"SKILLHUB_OUTDIR", outDir},
		{"SKILLHUB_SKILL_DIR", filepath.Join(workDir, ".claude", "skills")},
		{"SKILLHUB_INPUT_DIR", inputDir(workDir)},
		{"SKILLHUB_DATASET_DIR", datasetDir(workDir)},
		{"SKILLHUB_ARTIFACT_DIR", artifactDir(outDir)},
		{"SKILLHUB_USER_PROMPT", req.TestCase.UserPrompt},
		{"SKILLHUB_SKILL_CONTENT_HASH", req.SkillVersion.ContentHash},
		{"SKILLHUB_SKILL_VERSION_ID", req.SkillVersion.SkillVersionID},
		{"SKILLHUB_TRACE_LEVEL", req.Trace.Level},
		{"SKILLHUB_ARTIFACT_MAX_BYTES", strconv.FormatInt(req.ResourceLimits.ArtifactFileBytes, 10)},
		{"HOME", workDir},
	}
	if req.Trace.IngestionURL != "" {
		pairs = append(pairs, kv{"SKILLHUB_TRACE_URL", req.Trace.IngestionURL})
	}
	if tb := req.ResourceLimits.TokenBudget; tb != nil {
		if tb.MaxInputTokens > 0 {
			pairs = append(pairs, kv{"SKILLHUB_MAX_INPUT_TOKENS", strconv.FormatInt(tb.MaxInputTokens, 10)})
		}
		if tb.MaxOutputTokens > 0 {
			pairs = append(pairs, kv{"SKILLHUB_MAX_OUTPUT_TOKENS", strconv.FormatInt(tb.MaxOutputTokens, 10)})
		}
	}
	if req.Runtime.Model != "" {
		pairs = append(pairs, kv{"SKILLHUB_MODEL", req.Runtime.Model})
	}
	if g := req.ModelGateway; g != nil {
		pairs = append(pairs, kv{"ANTHROPIC_BASE_URL", g.BaseURL})
		if g.VirtualKey != "" {
			pairs = append(pairs, kv{"ANTHROPIC_AUTH_TOKEN", g.VirtualKey})
		}
	}
	// The one memory lever available on every platform (package doc, point 2):
	// a request to the V8 heap, not an OS wall.
	//
	// ponytail: this replaces NODE_OPTIONS wholesale rather than merging with
	// whatever the host operator may already have set; add merging if a
	// deployment ever needs both at once.
	if mb := req.ResourceLimits.MemoryBytes / (1 << 20); mb > 0 {
		pairs = append(pairs, kv{"NODE_OPTIONS", fmt.Sprintf("--max-old-space-size=%d", mb)})
	}

	set := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		set[p.k] = true
	}
	out := make([]string, 0, len(pairs)+len(hostEnvAllowlist))
	for _, raw := range os.Environ() {
		k, _, ok := strings.Cut(raw, "=")
		if !ok || k == "" {
			continue // Windows' "=C:=C:\..." drive-cwd entries have no name
		}
		if set[k] {
			continue // this driver sets it below; the host's value is not carried
		}
		if !hostEnvAllowed(k) {
			continue
		}
		out = append(out, raw)
	}
	for _, p := range pairs {
		out = append(out, p.k+"="+p.v)
	}
	return out
}

// hostEnvAllowlist is every host variable a workload inherits: the ones node
// itself needs to start and to find a temporary directory, on Windows and on
// POSIX. Names are upper-cased because Windows' are case-insensitive and Go
// reports them however the process block spells them.
//
// Nothing here carries a credential or a destination. Adding a name is a
// decision about what an unsandboxed workload gets to read, so it belongs in a
// diff somebody reviews rather than in whatever the operator's shell happened
// to export.
var hostEnvAllowlist = map[string]bool{
	"PATH":        true, // find node's own dependents; on Windows, its DLLs
	"PATHEXT":     true, // Windows: which extensions count as executable
	"COMSPEC":     true, // Windows: child_process.exec's shell
	"SYSTEMROOT":  true, // Windows: node will not start without it (crypto, winsock)
	"WINDIR":      true, // Windows: older spelling of the same thing
	"SYSTEMDRIVE": true, // Windows: path resolution for drive-relative paths
	"TEMP":        true, // os.tmpdir() on Windows
	"TMP":         true, // os.tmpdir() on Windows
	"TMPDIR":      true, // os.tmpdir() on POSIX
	"HOME":        true, // POSIX; overridden by this driver to the run's workDir
	"USERPROFILE": true, // Windows os.homedir()
	"LANG":        true, // locale for anything the workload shells out to
	"LC_ALL":      true,
}

// hostEnvAllowed also lets through the rest of the LC_* family, which is a
// family rather than a list (LC_CTYPE, LC_NUMERIC, and a platform's own
// additions) and carries nothing but locale names.
func hostEnvAllowed(name string) bool {
	upper := strings.ToUpper(name)
	return hostEnvAllowlist[upper] || strings.HasPrefix(upper, "LC_")
}

// Rootless reports whether this driver's workloads run without administrative
// privilege, which for a host process means: whether sandboxd itself does.
// There is no separate account to drop to — Start does not set
// SysProcAttr.Credential and has no privilege to drop with — so the honest
// answer is a detection of the current process, made per-platform in
// tree_unix.go / tree_windows.go (rootless()).
//
// Answering false takes the node out of dispatch rotation
// (schedule.go refuses a provider that does not run workloads unprivileged),
// and that is the intended outcome, not a bug to work around: a clean-mode
// node running elevated is one where an untrusted workload would have the
// administrator's own reach.
func (d *Driver) Rootless() bool { return rootless() }
