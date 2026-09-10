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

const logTailBytes = 32 << 10

type Config struct {
	NodeBin string

	RunnerScript string

	BaseDir string
}

type run struct {
	cmd     *exec.Cmd
	tree    processTree
	tail    *tailWriter
	workDir string
	outDir  string

	done    chan struct{}
	outcome sandbox.Outcome
	waitErr error
}

type Driver struct {
	cfg Config

	mu   sync.Mutex
	runs map[string]*run
}

var _ sandbox.Driver = (*Driver)(nil)

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

func (d *Driver) paths(id string) (workDir, outDir string) {
	root := filepath.Join(d.cfg.BaseDir, id)
	return filepath.Join(root, "work"), filepath.Join(root, "out")
}

func (d *Driver) get(id string) *run {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs[id]
}

func (d *Driver) take(id string) *run {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.runs[id]
	delete(d.runs, id)
	return r
}

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

		_ = cmd.Process.Kill()
		_ = tree.release()
		return fmt.Errorf("bind workload to its reap boundary: %w", err)
	}

	d.mu.Lock()
	d.runs[id] = r
	d.mu.Unlock()

	go d.reap(r)

	if err := d.pushInputs(ctx, r, req); err != nil {
		return err
	}
	return nil
}

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

func (d *Driver) Stop(ctx context.Context, id string, grace time.Duration) error {
	r := d.get(id)
	if r == nil {
		return nil
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

	}
	_ = os.RemoveAll(filepath.Dir(r.workDir))
	return nil
}

func (d *Driver) Adopt(ctx context.Context) ([]sandbox.Adopted, error) {
	return nil, nil
}

func (d *Driver) Healthy(ctx context.Context) bool {
	if _, err := exec.LookPath(d.cfg.NodeBin); err != nil {
		return false
	}
	return exec.CommandContext(ctx, d.cfg.NodeBin, "--version").Run() == nil
}

type ResourceEnforcement struct {
	Memory    bool
	Processes bool

	CPU bool

	Disk bool

	OpenFiles bool
}

func (d *Driver) ResourceEnforcement() ResourceEnforcement {
	return resourceEnforcement()
}

func (d *Driver) Reaping() Reaping {
	return reaping()
}

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
			continue
		}
		if set[k] {
			continue
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

var hostEnvAllowlist = map[string]bool{
	"PATH":        true,
	"PATHEXT":     true,
	"COMSPEC":     true,
	"SYSTEMROOT":  true,
	"WINDIR":      true,
	"SYSTEMDRIVE": true,
	"TEMP":        true,
	"TMP":         true,
	"TMPDIR":      true,
	"HOME":        true,
	"USERPROFILE": true,
	"LANG":        true,
	"LC_ALL":      true,
}

func hostEnvAllowed(name string) bool {
	upper := strings.ToUpper(name)
	return hostEnvAllowlist[upper] || strings.HasPrefix(upper, "LC_")
}

func (d *Driver) Rootless() bool { return rootless() }
