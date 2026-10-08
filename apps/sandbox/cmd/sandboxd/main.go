package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	networktypes "github.com/moby/moby/api/types/network"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/egress"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/mxcdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "egress-record" {
		if err := egress.Record(os.Stdin, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	token := os.Getenv("SKILLHUB_SANDBOX_TOKEN")
	if token == "" {

		log.Error("SKILLHUB_SANDBOX_TOKEN is required")
		os.Exit(1)
	}

	runtime := os.Getenv("SKILLHUB_SANDBOX_RUNTIME")
	image := envOr("SKILLHUB_SANDBOX_IMAGE", "skillhub/runtime-agent-sdk:2026.08-18")
	allowDevCmd := os.Getenv("SKILLHUB_SANDBOX_DEV_CMD") == "1"

	cleanMode := cleanNode(os.Getenv("SKILLHUB_CLEAN_MODE") == "1")
	if err := refuseDevSettings(runtime, image, allowDevCmd, bool(cleanMode)); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	mxcBin := os.Getenv("SKILLHUB_SANDBOX_MXC_BIN")
	kind, err := selectDriver(os.Getenv("SKILLHUB_SANDBOX_DRIVER"), runtime, mxcBin, cleanMode)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	node := openDriver(kind, mxcBin, dockerdrv.Config{
		Image:        image,
		Runtime:      runtime,
		Network:      envOr("SKILLHUB_SANDBOX_NETWORK", networktypes.NetworkNone),
		UID:          envInt("SKILLHUB_SANDBOX_UID", defaultWorkloadUserID),
		GID:          envInt("SKILLHUB_SANDBOX_GID", defaultWorkloadUserID),
		StorageQuota: os.Getenv("SKILLHUB_SANDBOX_STORAGE_QUOTA") == "1",
		AllowDevCmd:  allowDevCmd,
		Log:          log,
	}, log)
	drv := node.drv

	m := newRunManager(node, kind, cleanMode, log)

	probe := residentP02Probe()
	if err := errors.Join(refuseUndialableTargets(probe), refuseUnprobedProduction(runtime, probe)); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	probeCtx, stopProbe := context.WithCancel(context.Background())

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m = m.WithTrace(&sandbox.HTTPTraceSink{}, sandbox.NewMetrics(registry))

	if err := adoptBeforeProtection(func() error { return m.Adopt(context.Background()) }, func() {
		m = m.WithP02(probeCtx, probe)
	}); err != nil {
		log.Error("could not reconcile existing sandboxes", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              envOr("SKILLHUB_SANDBOX_ADDR", ":9000"),
		Handler:           (&sandbox.Server{M: m, Token: token, Metrics: registry}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveUntilSignalled(srv, drv, log)
	stopProbe()
	_ = node.closer()
}

const (
	defaultWorkloadUserID     = 65532
	defaultP02IntervalSeconds = 300
	defaultP02TimeoutSeconds  = 30
)

func newRunManager(node openedDriver, kind string, cleanMode cleanNode, log *slog.Logger) *sandbox.Manager {
	modes, egressAllow, egressUnenforced := declaredEgress(kind, cleanMode, log)

	return sandbox.NewManager(node.drv, sandbox.Config{
		Provider: envOr("SKILLHUB_SANDBOX_PROVIDER", "self_hosted"),
		Runtimes: []sandbox.RuntimeCapability{{
			Runtime:          "claude_agent_sdk",
			Versions:         []string{envOr("SKILLHUB_SANDBOX_RUNTIME_VERSION", "0.3.233")},
			AgentIntegration: []string{"in_sandbox_sdk"},
		}},
		MaxResources:             node.maxResources,
		MaxResourcesUnenforced:   node.unenforced,
		ReapsDetachedDescendants: node.reapsDetached,
		EgressModes:              modes,
		EgressAllow:              egressAllow,
		EgressUnenforced:         egressUnenforced,
		Slots:                    envInt("SKILLHUB_SANDBOX_SLOTS", 2),
		ResultRetention:          time.Duration(envInt("SKILLHUB_SANDBOX_RESULT_RETENTION_SECONDS", 0)) * time.Second,
	}, log)
}

func residentP02Probe() *sandbox.P02Probe {
	return sandbox.NewP02Probe(
		splitList(os.Getenv("SKILLHUB_SANDBOX_P02_TARGETS")),
		time.Duration(envInt("SKILLHUB_SANDBOX_P02_INTERVAL_SECONDS", defaultP02IntervalSeconds))*time.Second,
		time.Duration(envInt("SKILLHUB_SANDBOX_P02_TIMEOUT_SECONDS", defaultP02TimeoutSeconds))*time.Second,
	)
}

func declaredEgress(kind string, cleanMode cleanNode, log *slog.Logger) ([]string, []sandbox.EgressDestination, bool) {
	network := os.Getenv("SKILLHUB_SANDBOX_NETWORK")
	egressAllow := egressAllowFor(network, log)
	modes := sandbox.EgressModesFor(network, egressAllow)
	if kind == driverMXC {
		modes = []string{sandbox.EgressModeNone}
	}
	if len(egressAllow) == 0 && network != "" && network != networktypes.NetworkNone {

		log.Warn("no egress destination is rendered, so this node declares no egress route",
			"network", network, "modes", modes)
	}

	egressUnenforced := false
	if cleanMode {
		egressUnenforced = true
		modes = []string{sandbox.EgressModeDefaultDeny, sandbox.EgressModeNone}
		log.Warn("clean mode: egress modes are declared so runs can be scheduled, but this driver enforces none of them; " +
			"the workload reaches whatever this host reaches, and the run's allow list is what the user agreed to, not a boundary")
	}
	return modes, egressAllow, egressUnenforced
}

func serveUntilSignalled(srv *http.Server, drv sandbox.NodeCapabilities, log *slog.Logger) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	log.Info("sandbox provider listening", "addr", srv.Addr, "isolation", drv.Isolation())
	err := serveUntil(ctx, srv, srv.ListenAndServe)
	stop()
	if err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

const shutdownGrace = 15 * time.Second

func serveUntil(ctx context.Context, srv *http.Server, serve func() error) error {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-drained
	return nil
}

type openedDriver struct {
	drv           sandbox.Driver
	closer        func() error
	maxResources  sandbox.ResourceLimits
	unenforced    []string
	reapsDetached bool
}

func openDriver(kind, mxcBin string, docker dockerdrv.Config, log *slog.Logger) openedDriver {
	switch kind {
	case driverMXC:
		d := mxcDriver(mxcBin, log)
		return openedDriver{
			drv: d, closer: d.Close, maxResources: sandbox.DefaultLimits,
			unenforced: unenforcedCeilings(d.ResourceEnforcement()), reapsDetached: d.Reaping().Detached,
		}
	case driverLocal:
		d := localDriver(log)
		return openedDriver{
			drv: d, closer: d.Close, maxResources: cleanModeMaxResources(d.ResourceEnforcement(), log),
			unenforced: unenforcedCeilings(d.ResourceEnforcement()), reapsDetached: d.Reaping().Detached,
		}
	default:
		d, err := dockerdrv.New(docker)
		if err != nil {
			log.Error("docker driver unavailable", "err", err)
			os.Exit(1)
		}
		return openedDriver{drv: d, closer: d.Close, maxResources: sandbox.DefaultLimits, reapsDetached: true}
	}
}

func localDriver(log *slog.Logger) *localdrv.Driver {
	script, err := cleanModeRunnerScript()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	d, err := localdrv.New(localdrv.Config{RunnerScript: script})
	if err != nil {
		log.Error("local driver unavailable", "err", err)
		os.Exit(1)
	}
	return d
}

func egressAllowFor(network string, log *slog.Logger) []sandbox.EgressDestination {
	if network == "" || network == networktypes.NetworkNone {
		return nil
	}
	path := os.Getenv("SKILLHUB_SANDBOX_EGRESS_ALLOW")
	if path == "" {
		log.Error("SKILLHUB_SANDBOX_EGRESS_ALLOW is required when SKILLHUB_SANDBOX_NETWORK is set",
			"network", network,
			"hint", "render it: python3 tools/egress/render.py --out infra/egress/rendered")
		os.Exit(1)
	}
	egressAllow, err := sandbox.LoadEgressAllow(path)
	if err != nil {
		log.Error("could not load the rendered egress allow list", "path", path, "err", err)
		os.Exit(1)
	}
	return egressAllow
}

func adoptBeforeProtection(adopt func() error, protect func()) error {
	if err := adopt(); err != nil {
		return err
	}
	protect()
	return nil
}

type cleanNode bool

func refuseDevSettings(runtime, image string, allowDevCmd, cleanMode bool) error {
	if runtime != dockerdrv.UserSpaceKernelRuntime {
		return nil
	}
	switch {
	case cleanMode:

		return errors.New("SKILLHUB_CLEAN_MODE must not be set with runsc: a runsc node is never a clean node, " +
			"and both being set means some configuration was copied from a machine this is not")
	case !strings.Contains(image, "@sha256:"):
		return errors.New("SKILLHUB_SANDBOX_IMAGE must use an immutable digest with runsc")
	case allowDevCmd:
		return errors.New("SKILLHUB_SANDBOX_DEV_CMD must not be set with runsc: a caller-chosen entrypoint replaces the harness, and with it the run's token ceiling and its trace")
	}
	return nil
}

func refuseUndialableTargets(probe *sandbox.P02Probe) error {
	if len(probe.Skipped) == 0 {
		return nil
	}
	return fmt.Errorf("SKILLHUB_SANDBOX_P02_TARGETS has entries that are not host:port (%s): "+
		"a target the probe cannot dial is a check that never runs", strings.Join(probe.Skipped, ", "))
}

func refuseUnprobedProduction(runtime string, probe *sandbox.P02Probe) error {
	if runtime != dockerdrv.UserSpaceKernelRuntime || probe.Configured() {
		return nil
	}
	return errors.New("SKILLHUB_SANDBOX_P02_TARGETS must name the addresses a sandbox must not reach " +
		"(host:port, comma separated) when running under runsc: the P-02 block must be " +
		"verified by a resident probe, and a node with no targets reports not_configured forever")
}

func driverKind(cleanMode cleanNode) string {
	if cleanMode {
		return driverLocal
	}
	return driverDocker
}

const (
	driverDocker = "docker"
	driverMXC    = "mxc"
	driverLocal  = "local"
)

func selectDriver(requested, runtime, mxcBin string, cleanMode cleanNode) (string, error) {
	switch requested {
	case "":
		return driverKind(cleanMode), nil
	case driverDocker:
		if cleanMode {
			return "", errors.New("SKILLHUB_SANDBOX_DRIVER=docker cannot run with SKILLHUB_CLEAN_MODE=1: " +
				"clean mode runs workloads as plain host processes, not containers; " +
				"unset SKILLHUB_CLEAN_MODE to run containers, or set SKILLHUB_SANDBOX_DRIVER=local")
		}
	case driverLocal:
		if !cleanMode {
			return "", errors.New("SKILLHUB_SANDBOX_DRIVER=local requires SKILLHUB_CLEAN_MODE=1: " +
				"the local driver runs workloads as host processes with no isolation, so it only starts on a node that declares itself clean; " +
				"set SKILLHUB_CLEAN_MODE=1, or choose docker or mxc")
		}
	case driverMXC:
		if err := refuseMXCSettings(runtime, mxcBin, bool(cleanMode)); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("SKILLHUB_SANDBOX_DRIVER=%q is not a driver this node has: "+
			"set docker, mxc or local, or leave it empty to pick docker (local under SKILLHUB_CLEAN_MODE=1)", requested)
	}
	return requested, nil
}

func refuseMXCSettings(runtime, mxcBin string, cleanMode bool) error {
	switch {
	case runtime == dockerdrv.UserSpaceKernelRuntime:
		return errors.New("SKILLHUB_SANDBOX_DRIVER=mxc cannot run with SKILLHUB_SANDBOX_RUNTIME=runsc: " +
			"runsc is the docker driver's container runtime, and an mxc node would run without gVisor while its settings still name it; " +
			"unset SKILLHUB_SANDBOX_RUNTIME, or set SKILLHUB_SANDBOX_DRIVER=docker")
	case cleanMode:
		return errors.New("SKILLHUB_SANDBOX_DRIVER=mxc cannot run with SKILLHUB_CLEAN_MODE=1: " +
			"clean mode selects the unisolated local driver and declares egress it does not enforce; " +
			"unset SKILLHUB_CLEAN_MODE to run under mxc, or set SKILLHUB_SANDBOX_DRIVER=local")
	case mxcBin == "":
		return errors.New("SKILLHUB_SANDBOX_MXC_BIN is required with SKILLHUB_SANDBOX_DRIVER=mxc: " +
			"set it to the path of the mxc executable (lxc-exec on Linux, wxc-exec.exe on Windows); " +
			"it is never guessed or looked up on PATH")
	}
	return nil
}

func mxcDriver(mxcBin string, log *slog.Logger) *mxcdrv.Driver {
	script, err := cleanModeRunnerScript()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	d, err := mxcdrv.New(mxcdrv.Config{MXCBin: mxcBin, RunnerScript: script})
	if err != nil {
		log.Error("mxc driver unavailable", "err", err)
		os.Exit(1)
	}
	return d
}

func cleanModeRunnerScript() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("clean mode cannot locate run.mjs: this binary carries no source path, so it was not built from this repository")
	}
	// Repo root is four directories up from this source file's build path.
	return runnerScriptUnder(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

func runnerScriptUnder(repoRoot string) (string, error) {
	script := filepath.Join(repoRoot, "infra", "images", "runtime-agent-sdk", "run.mjs")
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("clean mode cannot find the workload script at %s (derived from this binary's build path): %w", script, err)
	}
	return script, nil
}

func cleanModeMaxResources(enf localdrv.ResourceEnforcement, log *slog.Logger) sandbox.ResourceLimits {
	limits := sandbox.DefaultLimits
	if !enf.Memory {
		log.Warn("clean mode: memory_bytes is declared for contract compatibility but is not OS-enforced on this platform (node --max-old-space-size only)",
			"memory_bytes", limits.MemoryBytes)
	}
	if !enf.Processes {
		log.Warn("clean mode: max_pids is declared for contract compatibility but is not OS-enforced on this platform",
			"max_pids", limits.MaxPIDs)
	}
	return limits
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

func osCeilings(enf localdrv.ResourceEnforcement) map[string]bool {
	return map[string]bool{
		"vcpu":           enf.CPU,
		"memory_bytes":   enf.Memory,
		"disk_bytes":     enf.Disk,
		"max_pids":       enf.Processes,
		"max_open_files": enf.OpenFiles,
	}
}

var heldElsewhere = map[string]bool{
	"wall_clock_soft_seconds": true,
	"wall_clock_hard_seconds": true,
	"artifact_total_bytes":    true,
	"artifact_file_bytes":     true,
	"token_budget":            true,
}

func unenforcedCeilings(enf localdrv.ResourceEnforcement) []string {
	held := osCeilings(enf)
	var out []string
	for _, name := range resourceLimitNames() {
		if heldElsewhere[name] || held[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// resourceLimitNames reads field names from the struct's json tags via
// reflection, so it stays in sync as ResourceLimits gains fields.
func resourceLimitNames() []string {
	t := reflect.TypeOf(sandbox.DefaultLimits)
	out := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	return out
}
