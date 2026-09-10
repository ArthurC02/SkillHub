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

var (
	fastBuckets  = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
	runBuckets   = []float64{1, 5, 15, 30, 60, 120, 300, 600, 900, 1800}
	traceBuckets = []float64{0.25, 0.5, 1, 2, 3, 5, 10, 30, 60}
)

var (
	SearchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillhub_search_duration_seconds",
		Help:    "Intent search latency by retrieval mode (DISC-001, NFR-004).",
		Buckets: fastBuckets,
	}, []string{"mode"})
)

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

	RunQueueDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "skillhub_run_queue_duration_seconds",
		Help:    "Time from run creation to the first provisioning transition (O11Y-001).",
		Buckets: runBuckets,
	})

	RunDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillhub_run_duration_seconds",
		Help:    "Time from run creation to its terminal state (O11Y-001).",
		Buckets: runBuckets,
	}, []string{"status"})
)

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

	CleanupBacklog = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "skillhub_run_cleanup_backlog",
		Help: "Terminal runs whose cleanup has not completed (O11Y-003).",
	})

	GatewayRevokeFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_gateway_revoke_failed_total",
		Help: "Virtual Key revocations that failed during cleanup (SBX-012, ADR-022 X-03/X-04 6b).",
	})
	SandboxDestroyFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_sandbox_destroy_failed_total",
		Help: "Sandbox teardowns that failed during cleanup (SBX-012, ADR-022 X-03/X-04).",
	}, []string{"provider"})

	RunTokenCeilingBreached = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_run_token_ceiling_breached_total",
		Help: "Runs the worker stopped for passing their token ceiling (PDM-005 §5.2a-4).",
	})
	RunTokenUsageUnreadable = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_run_token_usage_unreadable_total",
		Help: "Token-ceiling checks that could not read the gateway and let the run continue (PDM-005 §5.2a-4).",
	})

	OrphanObjectQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "skillhub_orphan_object_queue_depth",
		Help: "Package object keys awaiting collection after the last sweep (04 丙-73).",
	})
)

const (
	CeilingUpload   = "upload"
	CeilingURL      = "url"
	CeilingProduced = "produced"
)

var PackageSizeRefused = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "skillhub_package_size_refused_total",
	Help: "Packages refused for exceeding a size ceiling, by which ceiling (03:INGEST-016, 05 R-13).",
}, []string{"ceiling"})

const (
	RouteImportUpload = "skills_import_upload"
	RouteImportURL    = "skills_import_url"
	RouteGenerate     = "skills_generate"
	RoutePublicSearch = "public_search"
	RouteCatalog      = "catalog_browse"
)

var RateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "skillhub_rate_limited_total",
	Help: "Requests refused with 429 by the per-IP token bucket, by route (02:NFR-001 clause 5).",
}, []string{"route"})

var OutboxDeadLettered = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "skillhub_outbox_dead_lettered_total",
	Help: "Domain events isolated after repeated delivery failure (ADR-008 Poison Message).",
}, []string{"event_type"})

var OutboxDeadLetteredCurrent = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "skillhub_outbox_dead_lettered",
	Help: "Domain events currently sitting isolated in the outbox, awaiting a human (ADR-008).",
})

var (
	ProviderCapability = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_provider_capability_total",
		Help: "GET /capability attempts by outcome (O11Y-002, RUN-005).",
	}, []string{"provider", "result"})

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

var (
	OrphanScan = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_orphan_scan_total",
		Help: "Orphan scan passes per provider by outcome (RUN-007, O11Y-003).",
	}, []string{"provider", "result"})

	OrphanSandbox = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_orphan_sandbox_total",
		Help: "Sandboxes the orphan scan acted on, by outcome (O11Y-003).",
	}, []string{"provider", "action"})

	OrphanPersistent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "skillhub_orphan_sandbox_persistent",
		Help: "Leaked sandboxes present for two or more consecutive reconciler rounds (SBX-012, ADR-022 X-03).",
	}, []string{"provider"})

	ObjectsMissing = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "skillhub_storage_objects_missing",
		Help: "Rows whose stored object has been missing for two or more consecutive reconciler rounds (SEC-006, 04 丙-9).",
	}, []string{"resource_kind"})

	DispatchHalted = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "skillhub_dispatch_halted",
		Help: "1 while new Runs are not dispatched to this target (03:SEC-012, ADR-022 X-04).",
	}, []string{"target", "source"})
)

var (
	TraceEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skillhub_trace_events_total",
		Help: "Trace events offered to ingestion, by outcome (TRACE-008).",
	}, []string{"source", "result"})

	TraceMaskedFields = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skillhub_trace_masked_fields_total",
		Help: "Payload values replaced by the redaction placeholder (TRACE-005).",
	})

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

func ObserveSince(o prometheus.Observer, start time.Time) {
	o.Observe(time.Since(start).Seconds())
}

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

		slog.Error("metrics listener stopped", "error", err)
	}
}
