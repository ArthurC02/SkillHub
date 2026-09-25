package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestStatusClassSplitsEveryRangeTheProviderRecorderActsOn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   string
	}{
		{"no response reached the provider at all", 0, "error"},
		{"the throttle code is not lumped in with client errors", http.StatusTooManyRequests, "throttled"},
		{"a plain success", http.StatusOK, "ok"},
		{"just below the client-error boundary", 399, "ok"},
		{"on the client-error boundary", 400, "client_error"},
		{"just below the server-error boundary", 499, "client_error"},
		{"on the server-error boundary", 500, "server_error"},
		{"the highest server error", 599, "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusClass(tc.status); got != tc.want {
				t.Errorf("StatusClass(%d) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

type recordedObservations struct{ values []float64 }

func (r *recordedObservations) Observe(value float64) { r.values = append(r.values, value) }

func TestObserveSinceMeasuresTheElapsedTimeNotTheWallClock(t *testing.T) {
	observer := &recordedObservations{}

	ObserveSince(observer, time.Now().Add(-90*time.Second))

	if len(observer.values) != 1 {
		t.Fatalf("want exactly one observation, got %d", len(observer.values))
	}
	if elapsed := observer.values[0]; elapsed < 90 || elapsed > 150 {
		t.Errorf("observed %g seconds for a start 90 seconds ago; a unix timestamp would land here too", elapsed)
	}
}

func scrape(t *testing.T) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the metrics handler answered %d", recorder.Code)
	}
	return recorder.Body.String()
}

func seriesOf(body, family string) []string {
	var series []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "#") && strings.HasPrefix(line, family) {
			series = append(series, line)
		}
	}
	return series
}

func TestEveryMeasurementTheObservabilityRequirementNamesReachesAScrape(t *testing.T) {
	for _, tc := range []struct {
		measurement string
		family      string
		labels      []string
		record      func()
	}{{
		measurement: "搜尋延遲",
		family:      "skillhub_search_duration_seconds",
		labels:      []string{`mode="hybrid"`},
		record:      func() { SearchDuration.WithLabelValues("hybrid").Observe(0.2) },
	}, {
		measurement: "Run 排隊時間",
		family:      "skillhub_run_queue_duration_seconds",
		record:      func() { RunQueueDuration.Observe(4) },
	}, {
		measurement: "Sandbox 建立時間",
		family:      "skillhub_provider_request_duration_seconds",
		labels:      []string{`operation="create_run"`, `provider="selfhosted"`},
		record: func() {
			ObserveSince(ProviderRequestDuration.WithLabelValues("selfhosted", "create_run"), time.Now())
		},
	}, {
		measurement: "成功率",
		family:      "skillhub_run_terminal_total",
		labels:      []string{`failure_class="none"`, `status="succeeded"`},
		record:      func() { RunTerminal.WithLabelValues("succeeded", "none").Inc() },
	}, {
		measurement: "逾時率",
		family:      "skillhub_run_terminal_total",
		labels:      []string{`failure_class="timeout"`, `status="failed"`},
		record:      func() { RunTerminal.WithLabelValues("failed", "timeout").Inc() },
	}, {
		measurement: "清理失敗率",
		family:      "skillhub_run_cleanup_total",
		labels:      []string{`result="failed"`},
		record:      func() { Cleanup.WithLabelValues("failed").Inc() },
	}, {
		measurement: "Provider 可用性",
		family:      "skillhub_provider_capability_total",
		labels:      []string{`provider="selfhosted"`, `result="unhealthy"`},
		record:      func() { ProviderCapability.WithLabelValues("selfhosted", "unhealthy").Inc() },
	}} {
		t.Run(tc.measurement, func(t *testing.T) {
			tc.record()

			series := seriesOf(scrape(t), tc.family)
			if len(series) == 0 {
				t.Fatalf("%s: nothing named %s reaches a scrape", tc.measurement, tc.family)
			}
			exposed := strings.Join(series, "\n")
			for _, label := range tc.labels {
				if !strings.Contains(exposed, label) {
					t.Errorf("%s: %s exposes no series carrying %s, so that rate cannot be split out:\n%s",
						tc.measurement, tc.family, label, exposed)
				}
			}
		})
	}
}
