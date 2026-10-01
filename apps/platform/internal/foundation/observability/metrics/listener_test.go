package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func settingOf(value string) func(string) string {
	return func(name string) string {
		if name == "METRICS_PROFILING" {
			return value
		}
		return ""
	}
}

func TestProfilingIsServedOnlyWhenTurnedOn(t *testing.T) {
	for _, tc := range []struct {
		setting string
		want    int
	}{{"", http.StatusNotFound}, {"0", http.StatusNotFound}, {"1", http.StatusOK}} {
		w := httptest.NewRecorder()
		listenerRoutes(settingOf(tc.setting)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
		if w.Code != tc.want {
			t.Errorf("METRICS_PROFILING=%q: /debug/pprof/ answered %d, want %d", tc.setting, w.Code, tc.want)
		}
	}
	w := httptest.NewRecorder()
	listenerRoutes(settingOf("")).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/metrics answered %d, want 200", w.Code)
	}
}
