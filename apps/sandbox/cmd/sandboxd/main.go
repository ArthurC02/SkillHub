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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	token := os.Getenv("SKILLHUB_SANDBOX_TOKEN")
	if token == "" {

		log.Error("SKILLHUB_SANDBOX_TOKEN is required")
		os.Exit(1)
	}

	runtime := os.Getenv("SKILLHUB_SANDBOX_RUNTIME")
	image := envOr("SKILLHUB_SANDBOX_IMAGE", "skillhub/runtime-agent-sdk:2026.08-8")
	allowDevCmd := os.Getenv("SKILLHUB_SANDBOX_DEV_CMD") == "1"

	cleanMode := os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
	if err := refuseDevSettings(runtime, image, allowDevCmd, cleanMode); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	var (
		drv          sandbox.Driver
		closer       func() error
		maxResources = sandbox.DefaultLimits

		unenforced    []string
		reapsDetached = true
	)
	switch driverKind(cleanMode) {
	case "local":
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
		drv, closer = d, d.Close
		maxResources = cleanModeMaxResources(d.ResourceEnforcement(), log)
		unenforced = unenforcedCeilings(d.ResourceEnforcement())
		reapsDetached = d.Reaping().Detached
	default:
		d, err := dockerdrv.New(dockerdrv.Config{
			Image:        image,
			Runtime:      runtime,
			Network:      envOr("SKILLHUB_SANDBOX_NETWORK", "none"),
			UID:          envInt("SKILLHUB_SANDBOX_UID", 65532),
			GID:          envInt("SKILLHUB_SANDBOX_GID", 65532),
			StorageQuota: os.Getenv("SKILLHUB_SANDBOX_STORAGE_QUOTA") == "1",
			AllowDevCmd:  allowDevCmd,
		})
		if err != nil {
			log.Error("docker driver unavailable", "err", err)
			os.Exit(1)
		}
		drv, closer = d, d.Close
	}

	defer func() { _ = closer() }()

	var egressAllow []sandbox.EgressDestination
	network := os.Getenv("SKILLHUB_SANDBOX_NETWORK")
	if network != "" && network != "none" {
		path := os.Getenv("SKILLHUB_SANDBOX_EGRESS_ALLOW")
		if path == "" {

			log.Error("SKILLHUB_SANDBOX_EGRESS_ALLOW is required when SKILLHUB_SANDBOX_NETWORK is set",
				"network", network,
				"hint", "render it: python3 tools/egress/render.py --out infra/egress/rendered")
			os.Exit(1)
		}
		var err error
		egressAllow, err = sandbox.LoadEgressAllow(path)
		if err != nil {
			log.Error("could not load the rendered egress allow list", "path", path, "err", err)
			os.Exit(1)
		}
	}
	modes := sandbox.EgressModesFor(network, egressAllow)
	if len(egressAllow) == 0 && network != "" && network != "none" {

		log.Warn("no egress destination is rendered, so this node declares no egress route",
			"network", network, "modes", modes)
	}

	egressUnenforced := false
	if cleanMode {
		egressUnenforced = true
		modes = []string{"default_deny", "none"}
		log.Warn("clean mode: egress modes are declared so runs can be scheduled, but this driver enforces none of them; " +
			"the workload reaches whatever this host reaches, and the run's allow list is what the user agreed to, not a boundary")
	}

	isolation := resolveIsolation(cleanMode, runtime)
	m := sandbox.NewManager(drv, sandbox.Config{
		Provider: envOr("SKILLHUB_SANDBOX_PROVIDER", "self_hosted"),
		Runtimes: []sandbox.RuntimeCapability{{
			Runtime:          "claude_agent_sdk",
			Versions:         []string{envOr("SKILLHUB_SANDBOX_RUNTIME_VERSION", "0.3.233")},
			AgentIntegration: []string{"in_sandbox_sdk"},
		}},
		MaxResources:             maxResources,
		MaxResourcesUnenforced:   unenforced,
		IsolationLevel:           isolation,
		ReapsDetachedDescendants: reapsDetached,
		EgressModes:              modes,
		EgressAllow:              egressAllow,
		EgressUnenforced:         egressUnenforced,
		Slots:                    envInt("SKILLHUB_SANDBOX_SLOTS", 2),
	}, log)

	probe := sandbox.NewP02Probe(
		splitList(os.Getenv("SKILLHUB_SANDBOX_P02_TARGETS")),
		time.Duration(envInt("SKILLHUB_SANDBOX_P02_INTERVAL_SECONDS", 300))*time.Second,
		time.Duration(envInt("SKILLHUB_SANDBOX_P02_TIMEOUT_SECONDS", 30))*time.Second,
	)
	if err := refuseUnprobedProduction(runtime, probe); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	probeCtx, stopProbe := context.WithCancel(context.Background())
	defer stopProbe()

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("sandbox provider listening", "addr", srv.Addr, "isolation", isolation)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func adoptBeforeProtection(adopt func() error, protect func()) error {
	if err := adopt(); err != nil {
		return err
	}
	protect()
	return nil
}

func refuseDevSettings(runtime, image string, allowDevCmd, cleanMode bool) error {
	if runtime != "runsc" {
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

func refuseUnprobedProduction(runtime string, probe *sandbox.P02Probe) error {
	if runtime != "runsc" || probe.Configured() {
		return nil
	}
	return errors.New("SKILLHUB_SANDBOX_P02_TARGETS must name the addresses a sandbox must not reach " +
		"(host:port, comma separated) when running under runsc: ADR-022 T10 requires the P-02 block to be " +
		"verified by a resident probe, and a node with no targets reports not_configured forever")
}

func driverKind(cleanMode bool) string {
	if cleanMode {
		return "local"
	}
	return "docker"
}

func resolveIsolation(cleanMode bool, runtime string) string {
	switch {
	case cleanMode:
		return "clean"
	case runtime == "runsc":
		return "gvisor"
	default:
		return "container"
	}
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
