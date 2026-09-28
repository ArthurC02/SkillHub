package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestProviderSeriesAreLabelledByProviderAndResult(t *testing.T) {
	for name, vec := range map[string]*prometheus.CounterVec{
		"provider capability": ProviderCapability,
		"orphan scan":         OrphanScan,
	} {
		if _, err := vec.GetMetricWith(prometheus.Labels{"provider": "p", "result": "ok"}); err != nil {
			t.Errorf("%s does not take provider and result labels: %v", name, err)
		}
	}
}

func TestCleanupAndTraceSeriesAreLabelledByResult(t *testing.T) {
	if _, err := Cleanup.GetMetricWith(prometheus.Labels{"result": "cleaned"}); err != nil {
		t.Errorf("cleanup does not take a result label: %v", err)
	}
	if _, err := TraceEvents.GetMetricWith(prometheus.Labels{"source": "s", "result": "stored"}); err != nil {
		t.Errorf("trace events do not take source and result labels: %v", err)
	}
}

func TestSandboxSeriesAreLabelledByProvider(t *testing.T) {
	if _, err := SandboxDestroyFailed.GetMetricWith(prometheus.Labels{"provider": "p"}); err != nil {
		t.Errorf("sandbox destroy failures do not take a provider label: %v", err)
	}
	if _, err := OrphanPersistent.GetMetricWith(prometheus.Labels{"provider": "p"}); err != nil {
		t.Errorf("persistent orphans do not take a provider label: %v", err)
	}
}
