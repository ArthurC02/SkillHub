package sandbox

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Dispatched  prometheus.Counter
	Finished    *prometheus.CounterVec
	Active      prometheus.Gauge
	TracePushes *prometheus.CounterVec
	TraceEvents prometheus.Counter
}

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

func MetricsHandler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
