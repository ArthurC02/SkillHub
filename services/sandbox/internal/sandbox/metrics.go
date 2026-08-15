package sandbox

// The execution node's own observability (O11Y-001/003, NFR-005). Separate from
// the platform's registry because this is a separate process on a separate
// machine, and separate from the Run Trace plane because nothing here is
// user-visible or sandbox-authored.
//
// Deliberately small. The platform already measures the run funnel end to end;
// what only this side can answer is what its own sandboxes and its own trace
// collector are doing. Labels are the provider's own vocabulary - a workload
// cannot create a series.

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the collector set for one provider process. It is a value rather
// than package-level state so a test can build a Manager without touching a
// global registry, and so two Managers in one test binary do not fight over one.
type Metrics struct {
	Dispatched  prometheus.Counter
	Finished    *prometheus.CounterVec
	Active      prometheus.Gauge
	TracePushes *prometheus.CounterVec
	TraceEvents prometheus.Counter
}

// NewMetrics registers the set. A nil registry means "not observed", which is
// what the manager tests want; every method below is nil-safe.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		return nil
	}
	factory := promauto.With(reg)
	return &Metrics{
		Dispatched: factory.NewCounter(prometheus.CounterOpts{
			Name: "skillhub_sandbox_dispatched_total",
			Help: "Attempts this provider created a sandbox for.",
		}),
		Finished: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "skillhub_sandbox_finished_total",
			Help: "Sandboxes reaching a terminal result, by result status.",
		}, []string{"status"}),
		Active: factory.NewGauge(prometheus.GaugeOpts{
			Name: "skillhub_sandbox_active",
			Help: "Sandboxes this provider currently holds resources for.",
		}),
		TracePushes: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "skillhub_sandbox_trace_push_total",
			Help: "Trace batches pushed to the control plane, by outcome (TRACE-002).",
		}, []string{"result"}),
		TraceEvents: factory.NewCounter(prometheus.CounterOpts{
			Name: "skillhub_sandbox_trace_events_total",
			Help: "Trace events accepted by the control plane (TRACE-002).",
		}),
	}
}

func (m *Metrics) dispatched() {
	if m != nil {
		m.Dispatched.Inc()
	}
}

func (m *Metrics) finished(status ResultStatus) {
	if m != nil {
		m.Finished.WithLabelValues(string(status)).Inc()
	}
}

func (m *Metrics) active(n int) {
	if m != nil {
		m.Active.Set(float64(n))
	}
}

func (m *Metrics) tracePush(result string, events int) {
	if m == nil {
		return
	}
	m.TracePushes.WithLabelValues(result).Inc()
	m.TraceEvents.Add(float64(events))
}

// MetricsHandler exposes the registry in Prometheus text format. It goes behind
// the same provider token as every other route: sandboxd has no unauthenticated
// path, and a scrape endpoint is not the place to start.
func MetricsHandler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
