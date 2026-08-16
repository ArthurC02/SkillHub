// Package metrics is the platform's own observability plane (O11Y-001~003,
// NFR-005). It is deliberately not the Run Trace plane of ADR-009: nothing here
// is derived from sandbox-supplied content, and nothing here is shown to a user.
//
// Two rules follow from that split and both are enforced by what the label sets
// below allow:
//
//  1. No user data in a label. run_id, workspace_id, user_id, skill names and
//     prompts are all unbounded cardinality *and* user data; they belong in logs
//     correlated by run_id (NFR-005), never in a metric series.
//  2. No label whose value comes from a sandbox. Alerting must not depend on
//     untrusted input (contracts/events/README.md §1), so provider names come
//     from deployment configuration and outcomes come from the platform's own
//     vocabulary.
//
// Exposition is Prometheus text on a separate listener (Serve), not on the
// public API mux: /metrics is an operator surface and putting it behind the
// same port as the SPA's API would make it internet-reachable by default.
package metrics

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Bucket sets. Three are enough: sub-second request work, whole-run work that
// runs to the PDM-005 wall clock of 900s, and the trace ingestion lag that
// NFR-004 wants inside 3 seconds.
var (
	fastBuckets  = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
	runBuckets   = []float64{1, 5, 15, 30, 60, 120, 300, 600, 900, 1800}
	traceBuckets = []float64{0.25, 0.5, 1, 2, 3, 5, 10, 30, 60}
)

// O11Y-001: search.
var (
	// SearchDuration is DISC-001 latency, split by which retrieval path answered.
	// NFR-004's target is p95 < 2s for indexed content.
	SearchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillhub_search_duration_seconds",
		Help:    "Intent search latency by retrieval mode (DISC-001, NFR-004).",
		Buckets: fastBuckets,
	}, []string{"mode"})
)

// O11Y-001: run funnel. Counters are keyed by the platform's own closed
// vocabularies - run_status and runs.failure_class - so a new provider or a new
// skill can never add a series.
var (
	RunCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_run_created_total",
		Help: "Runs accepted and queued (RUN-001).",
	})
	RunRefused = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_run_refused_total",
		Help: "Run requests refused before queueing, by reason (RUN-005).",
	}, []string{"reason"})
	RunTerminal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_run_terminal_total",
		Help: "Runs reaching a terminal state, by status and failure class (RUN-006).",
	}, []string{"status", "failure_class"})
	// RunQueueDuration is queued -> provisioning: how long a run waited for a
	// worker and a free provider slot.
	RunQueueDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "skillhub_run_queue_duration_seconds",
		Help:    "Time from run creation to the first provisioning transition (O11Y-001).",
		Buckets: runBuckets,
	})
	// RunDuration is creation -> terminal, the number the user actually feels.
	RunDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillhub_run_duration_seconds",
		Help:    "Time from run creation to its terminal state (O11Y-001).",
		Buckets: runBuckets,
	}, []string{"status"})
)

// O11Y-001 / O11Y-003: cleanup.
var (
	Cleanup = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_run_cleanup_total",
		Help: "Cleanup passes by outcome (RUN-007).",
	}, []string{"result"})
	CleanupDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "skillhub_run_cleanup_duration_seconds",
		Help:    "How long one run's cleanup took (RUN-007).",
		Buckets: fastBuckets,
	})
	// CleanupBacklog is the gauge the alert rules watch: terminal runs whose
	// sandbox has not been confirmed released. A backlog that only grows means
	// sandboxes are being paid for and nobody is releasing them.
	CleanupBacklog = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "skillhub_run_cleanup_backlog",
		Help: "Terminal runs whose cleanup has not completed (O11Y-003).",
	})
	// The two counters ADR-022 X-03/X-04 names, kept apart because the correct
	// response to each is the opposite of the other. A sandbox that will not die
	// holds a slot, so the fleet drains; a Virtual Key that will not revoke holds
	// no slot at all, and suspending dispatch would not un-leak it - that one needs
	// a human at the gateway. The aggregate Cleanup{result="failed"} above cannot
	// tell them apart, which is why alerting on it could only ever be provisional.
	GatewayRevokeFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_gateway_revoke_failed_total",
		Help: "Virtual Key revocations that failed during cleanup (SBX-012, ADR-022 X-03/X-04 6b).",
	})
	SandboxDestroyFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_sandbox_destroy_failed_total",
		Help: "Sandbox teardowns that failed during cleanup (SBX-012, ADR-022 X-03/X-04).",
	}, []string{"provider"})
)

// O11Y-002: provider health. `provider` comes from SKILLHUB_SANDBOX_PROVIDERS,
// which is deployment-static, so its cardinality is the size of the fleet.
var (
	ProviderCapability = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_provider_capability_total",
		Help: "GET /capability attempts by outcome (O11Y-002, RUN-005).",
	}, []string{"provider", "result"})
	// ProviderRequest counts every contract call by HTTP status class rather
	// than by exact code: 429 and 5xx are the two the retry policy and the
	// alert rules care about, and a code-per-series buys nothing else.
	ProviderRequest = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_provider_request_total",
		Help: "Sandbox provider calls by operation and status class (O11Y-002).",
	}, []string{"provider", "operation", "class"})
	ProviderRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillhub_provider_request_duration_seconds",
		Help:    "Sandbox provider call latency (O11Y-002).",
		Buckets: fastBuckets,
	}, []string{"provider", "operation"})
)

// O11Y-003: leaked sandboxes.
var (
	OrphanScan = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_orphan_scan_total",
		Help: "Orphan scan passes per provider by outcome (RUN-007, O11Y-003).",
	}, []string{"provider", "result"})
	// OrphanSandbox counts what a scan decided about individual sandboxes.
	// `destroyed` above zero is a leak that happened; `failed` is a leak that
	// is still there.
	OrphanSandbox = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_orphan_sandbox_total",
		Help: "Sandboxes the orphan scan acted on, by outcome (O11Y-003).",
	}, []string{"provider", "action"})
	// OrphanPersistent is ADR-022 X-03 as a number a rule can read: leaked
	// resources the reconciler has now seen in two or more *consecutive* rounds.
	//
	// A gauge and not a counter, because the question is "is it still there", and
	// counters cannot answer that - increase(orphan_sandbox_total) rising only says
	// something was acted on during the window, which two different resources
	// failing once each satisfies just as well as one resource stuck for an hour.
	// The consecutive-round bookkeeping is db/migrations/0021's sightings table.
	OrphanPersistent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "skillhub_orphan_sandbox_persistent",
		Help: "Leaked sandboxes present for two or more consecutive reconciler rounds (SBX-012, ADR-022 X-03).",
	}, []string{"provider"})
)

// Trace ingestion (TRACE-005, TRACE-008). Health of the pipeline itself, not of
// what travels through it.
var (
	// TraceEvents counts the outcome of each event offered to ingestion.
	// `duplicate` is expected under at-least-once delivery and is not an error;
	// `rejected` is a schema or scope violation and is.
	TraceEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_trace_events_total",
		Help: "Trace events offered to ingestion, by outcome (TRACE-008).",
	}, []string{"source", "result"})
	// TraceMaskedFields is how much the masker actually redacted. A sudden rise
	// means secrets are reaching the trace path; a drop to zero after a change
	// to the masker means it stopped working, which is the failure NFR-002 has
	// no other detector for.
	TraceMaskedFields = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_trace_masked_fields_total",
		Help: "Payload values replaced by the redaction placeholder (TRACE-005).",
	})
	// TraceIngestLag is occurred_at -> stored. NFR-004 targets 3 seconds from
	// event production to the run screen; this is the platform's half of it.
	TraceIngestLag = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "skillhub_trace_ingest_lag_seconds",
		Help:    "Producer timestamp to storage, per event (NFR-004).",
		Buckets: traceBuckets,
	})
	TraceIngestRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_trace_ingest_rejected_total",
		Help: "Ingestion requests refused before any event was read (TRACE-008).",
	}, []string{"reason"})
)

// StatusClass buckets an HTTP status into the three classes the alert rules
// distinguish. Transport failures have no status at all and get "error".
func StatusClass(status int) string {
	switch {
	case status == 0:
		return "error"
	case status == http.StatusTooManyRequests:
		return "throttled"
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "ok"
	}
}

// ObserveSince is the one-liner every call site uses; it exists so no call site
// has to remember to divide by time.Second.
func ObserveSince(o prometheus.Observer, start time.Time) {
	o.Observe(time.Since(start).Seconds())
}

// Serve exposes /metrics on its own listener and blocks until it stops. An
// empty addr disables it, which is what a deployment without a scraper wants -
// silently failing to bind would be worse than not listening at all.
//
// It is intentionally not mounted on the public API mux: this endpoint is an
// operator surface, and NFR-005 keeps the platform plane apart from anything a
// user can reach.
func Serve(addr string) {
	if addr == "" {
		slog.Info("metrics endpoint disabled (METRICS_ADDR unset)")
		return
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	slog.Info("metrics listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Not fatal: losing observability must not take the service down with it.
		slog.Error("metrics listener stopped", "error", err)
	}
}
