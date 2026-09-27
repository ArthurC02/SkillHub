package mxcdrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const (
	configName = "mxc.json"

	backendFailureExitCode = 127
	backendFailureCode     = "backend_error"

	healthCacheTTL = 30 * time.Second
	healthTimeout  = 15 * time.Second
)

var ErrLaunchFailed = errors.New("mxc could not start the workload")

type Config struct {
	MXCBin string

	NodeBin string

	RunnerScript string

	BaseDir string
}

type Driver struct {
	*localdrv.Driver

	mxcBin    string
	nodeBin   string
	runnerDir string

	healthMu           sync.Mutex
	healthy            bool
	healthCheckedUntil time.Time
}

var _ sandbox.Driver = (*Driver)(nil)

func New(cfg Config) (*Driver, error) {
	if cfg.MXCBin == "" {
		return nil, errors.New("an mxc executable path is required")
	}
	if info, err := os.Stat(cfg.MXCBin); err != nil || info.IsDir() {
		return nil, fmt.Errorf("mxc executable %s is not a file this node can run", cfg.MXCBin)
	}
	if cfg.NodeBin == "" {
		cfg.NodeBin = "node"
	}
	nodeBin, err := exec.LookPath(cfg.NodeBin)
	if err != nil {
		return nil, fmt.Errorf("node runtime not found (%s): %w", cfg.NodeBin, err)
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = filepath.Join(os.TempDir(), "skillhub-mxc")
	}
	d := &Driver{mxcBin: cfg.MXCBin, nodeBin: nodeBin, runnerDir: filepath.Dir(cfg.RunnerScript)}
	inner, err := localdrv.New(localdrv.Config{
		NodeBin:      nodeBin,
		RunnerScript: cfg.RunnerScript,
		BaseDir:      cfg.BaseDir,
		Launch:       d.launch,
	})
	if err != nil {
		return nil, err
	}
	d.Driver = inner
	return d, nil
}

func (d *Driver) Isolation() sandbox.IsolationStrength { return sandbox.IsolationWeak }

func (d *Driver) Wait(ctx context.Context, id string) (sandbox.Outcome, error) {
	out, err := d.Driver.Wait(ctx, id)
	if err == nil && backendFailed(out) {
		return out, ErrLaunchFailed
	}
	return out, err
}

func (d *Driver) Healthy(ctx context.Context) bool {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()
	if time.Now().Before(d.healthCheckedUntil) {
		return d.healthy
	}
	healthy := d.runtimeStartsInside(ctx)
	if ctx.Err() != nil {
		return false
	}
	d.healthy = healthy
	d.healthCheckedUntil = time.Now().Add(healthCacheTTL)
	return healthy
}

func (d *Driver) launch(direct *exec.Cmd, workDir, outDir string) (*exec.Cmd, error) {
	if direct.Err != nil {
		return nil, direct.Err
	}
	commandLine, err := joinCommandLine(append([]string{direct.Path}, direct.Args[1:]...))
	if err != nil {
		return nil, err
	}
	path := filepath.Join(filepath.Dir(workDir), configName)
	policy := runPolicy(commandLine, direct.Dir, []string{workDir, outDir}, []string{d.runnerDir})
	if err := writePolicy(path, policy); err != nil {
		return nil, err
	}
	cmd := exec.Command(d.mxcBin, path)
	cmd.Dir = direct.Dir
	cmd.Env = direct.Env
	return cmd, nil
}

func (d *Driver) runtimeStartsInside(ctx context.Context) bool {
	dir, err := os.MkdirTemp("", "skillhub-mxc-health-")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	commandLine, err := joinCommandLine([]string{d.nodeBin, "--version"})
	if err != nil {
		return false
	}
	path := filepath.Join(dir, configName)
	if err := writePolicy(path, runPolicy(commandLine, dir, []string{dir}, nil)); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.mxcBin, path)
	cmd.Dir = dir
	cmd.Env = hostBasics()
	return cmd.Run() == nil
}

func backendFailed(out sandbox.Outcome) bool {
	if out.ExitCode != backendFailureExitCode {
		return false
	}
	var report struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Output), &report); err != nil {
		return false
	}
	return report.Error.Code == backendFailureCode
}

type policy struct {
	Process    processPolicy    `json:"process"`
	Filesystem filesystemPolicy `json:"filesystem"`
	Network    networkPolicy    `json:"network"`
	Lifecycle  lifecyclePolicy  `json:"lifecycle"`
}

type processPolicy struct {
	CommandLine string `json:"commandLine"`
	Cwd         string `json:"cwd"`
}

type filesystemPolicy struct {
	ReadwritePaths []string `json:"readwritePaths"`
	ReadonlyPaths  []string `json:"readonlyPaths"`
}

type networkPolicy struct {
	DefaultPolicy string `json:"defaultPolicy"`
}

type lifecyclePolicy struct {
	DestroyOnExit bool `json:"destroyOnExit"`
}

func runPolicy(commandLine, cwd string, readwrite, readonly []string) policy {
	if readonly == nil {
		readonly = []string{}
	}
	return policy{
		Process:    processPolicy{CommandLine: commandLine, Cwd: cwd},
		Filesystem: filesystemPolicy{ReadwritePaths: readwrite, ReadonlyPaths: readonly},
		Network:    networkPolicy{DefaultPolicy: "block"},
		Lifecycle:  lifecyclePolicy{DestroyOnExit: true},
	}
}

func writePolicy(path string, p policy) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write the mxc policy: %w", err)
	}
	return nil
}

func joinCommandLine(args []string) (string, error) {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsRune(arg, '"') {
			return "", fmt.Errorf("a workload argument contains a double quote, which the mxc command line cannot carry: %s", arg)
		}
		if arg == "" || strings.ContainsAny(arg, " \t") {
			arg = `"` + arg + `"`
		}
		quoted[i] = arg
	}
	return strings.Join(quoted, " "), nil
}

var hostBasicNames = map[string]bool{
	"PATH":       true,
	"SYSTEMROOT": true,
	"WINDIR":     true,
	"TEMP":       true,
	"TMP":        true,
	"TMPDIR":     true,
}

func hostBasics() []string {
	var out []string
	for _, raw := range os.Environ() {
		name, _, ok := strings.Cut(raw, "=")
		if ok && hostBasicNames[strings.ToUpper(name)] {
			out = append(out, raw)
		}
	}
	return out
}
