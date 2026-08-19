// Command sandboxd serves the Sandbox Provider Port
// (contracts/openapi/sandbox-provider.yaml) for one execution node.
//
// It holds no database connection and needs none: the execution plane answers
// questions and never reaches into the control plane (iron rule 2). Everything
// it knows about a run arrives in the RunRequest or is read back from the
// sandbox it created.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	token := os.Getenv("SKILLHUB_SANDBOX_TOKEN")
	if token == "" {
		// Fail closed. A provider that serves without a token is one any
		// process on the node can hand untrusted code to.
		log.Error("SKILLHUB_SANDBOX_TOKEN is required")
		os.Exit(1)
	}

	runtime := os.Getenv("SKILLHUB_SANDBOX_RUNTIME") // "runsc" in production
	image := envOr("SKILLHUB_SANDBOX_IMAGE", "skillhub/runtime-agent-sdk:2026.08-3")
	if runtime == "runsc" && !strings.Contains(image, "@sha256:") {
		log.Error("SKILLHUB_SANDBOX_IMAGE must use an immutable digest with runsc")
		os.Exit(1)
	}
	drv, err := dockerdrv.New(dockerdrv.Config{
		Image:        image,
		Runtime:      runtime,
		Network:      envOr("SKILLHUB_SANDBOX_NETWORK", "none"),
		UID:          envInt("SKILLHUB_SANDBOX_UID", 65532),
		GID:          envInt("SKILLHUB_SANDBOX_GID", 65532),
		StorageQuota: os.Getenv("SKILLHUB_SANDBOX_STORAGE_QUOTA") == "1",
		AllowDevCmd:  os.Getenv("SKILLHUB_SANDBOX_DEV_CMD") == "1",
	})
	if err != nil {
		log.Error("docker driver unavailable", "err", err)
		os.Exit(1)
	}
	defer drv.Close()

	// The declared isolation level follows the runtime actually configured.
	// Declaring gvisor on a machine running runc would be a claim the provider
	// cannot keep, and RUN-005 dispatches on this answer.
	isolation := "container"
	if runtime == "runsc" {
		isolation = "gvisor"
	}
	m := sandbox.NewManager(drv, sandbox.Config{
		Provider: envOr("SKILLHUB_SANDBOX_PROVIDER", "self_hosted"),
		Runtimes: []sandbox.RuntimeCapability{{
			Runtime:          "claude_agent_sdk",
			Versions:         []string{envOr("SKILLHUB_SANDBOX_RUNTIME_VERSION", "0.3.233")},
			AgentIntegration: []string{"in_sandbox_sdk"},
		}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: isolation,
		EgressModes:    egressModes(os.Getenv("SKILLHUB_SANDBOX_NETWORK")),
		Slots:          envInt("SKILLHUB_SANDBOX_SLOTS", 2),
	}, log)

	// TRACE-002: the sandbox has no network, so this process is what carries its
	// trace events to the control plane. The destination is per run and arrives
	// in the RunRequest; all that is configured here is the ability to push.
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m = m.WithTrace(&sandbox.HTTPTraceSink{}, sandbox.NewMetrics(registry))

	// Sandboxes outlive this process. Rebuilding from labels before serving
	// keeps a restarted provider from answering 404 for live attempts and from
	// reporting an empty GET /runs, which an orphan scan reads as "nothing
	// leaked" (RUN-007, RUN-008).
	if err := m.Adopt(context.Background()); err != nil {
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
		// Running sandboxes are deliberately left alone: they are the
		// platform's to cancel or destroy, and this process will adopt them
		// again on the way back up.
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("sandbox provider listening", "addr", srv.Addr, "isolation", isolation)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// egressModes reports what this deployment can actually offer. "none" is the
// dev baseline and is stronger than default_deny, not weaker; a deployment with
// an egress proxy network names it and gets both.
func egressModes(network string) []string {
	if network == "" || network == "none" {
		return []string{"none"}
	}
	return []string{"default_deny", "none"}
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
