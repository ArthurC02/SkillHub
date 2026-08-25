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
	allowDevCmd := os.Getenv("SKILLHUB_SANDBOX_DEV_CMD") == "1"
	if err := refuseDevSettings(runtime, image, allowDevCmd); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	drv, err := dockerdrv.New(dockerdrv.Config{
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
	defer drv.Close()

	// What this node routes to, rendered from infra/egress/allowlist.yaml by
	// tools/egress/render.py in the same pass that produced its nftables ruleset
	// and its resolver config. Loading it here is what lets accept() refuse a
	// destination the ruleset has no rule for (ADR-022 A1-e) instead of
	// dispatching the run and letting it time out.
	//
	// Required only when this node has an egress network at all. A node with no
	// network already refuses every allow list on the older, coarser check, so
	// demanding the file there would fail nodes that are correctly configured
	// for `none`.
	var egressAllow []sandbox.EgressDestination
	network := os.Getenv("SKILLHUB_SANDBOX_NETWORK")
	if network != "" && network != "none" {
		path := os.Getenv("SKILLHUB_SANDBOX_EGRESS_ALLOW")
		if path == "" {
			// Fail closed at startup rather than at the first dispatch. A node
			// with a network and no rendered list would advertise an egress
			// route and refuse every destination sent to it, which the scheduler
			// reads as a node to keep trying.
			log.Error("SKILLHUB_SANDBOX_EGRESS_ALLOW is required when SKILLHUB_SANDBOX_NETWORK is set",
				"network", network,
				"hint", "render it: python3 tools/egress/render.py --out infra/egress/rendered")
			os.Exit(1)
		}
		egressAllow, err = sandbox.LoadEgressAllow(path)
		if err != nil {
			log.Error("could not load the rendered egress allow list", "path", path, "err", err)
			os.Exit(1)
		}
	}
	modes := sandbox.EgressModesFor(network, egressAllow)
	if len(egressAllow) == 0 && network != "" && network != "none" {
		// Not an error: an allow-list whose pinned_ip is still `unset` renders
		// no destination on purpose. But the node must not go on advertising a
		// route it cannot take, so it declares `none` and says why once.
		log.Warn("no egress destination is rendered, so this node declares no egress route",
			"network", network, "modes", modes)
	}

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
		EgressModes:    modes,
		EgressAllow:    egressAllow,
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

// refuseDevSettings fails a production node closed when it carries a
// development setting. `runsc` is the only signal available this early that
// this is production (ADR-015), and both of these switch off something a user
// was already promised: an unpinned image means the run record cannot say what
// actually ran (I-02), and a caller-chosen entrypoint replaces the image's own,
// so run.mjs never starts - with it go the harness's token ceiling (PDM-005
// 5.2a) and every TRACE-002 event, while the run's Virtual Key is injected as
// usual. A systemd unit copied from a dev template is all it takes.
func refuseDevSettings(runtime, image string, allowDevCmd bool) error {
	if runtime != "runsc" {
		return nil
	}
	switch {
	case !strings.Contains(image, "@sha256:"):
		return errors.New("SKILLHUB_SANDBOX_IMAGE must use an immutable digest with runsc")
	case allowDevCmd:
		return errors.New("SKILLHUB_SANDBOX_DEV_CMD must not be set with runsc: a caller-chosen entrypoint replaces the harness, and with it the run's token ceiling and its trace")
	}
	return nil
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
