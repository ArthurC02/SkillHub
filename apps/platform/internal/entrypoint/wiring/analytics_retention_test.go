package wiring

import (
	"strings"
	"testing"
	"time"
)

func TestAnAnalyticsRetentionIsOffUnsetAndRefusedWhenSetButUnusable(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        time.Duration
		refused     bool
	}{
		{"unset turns collection off", "", 0, false},
		{"a positive duration is used", "2160h", 2160 * time.Hour, false},
		{"one second is the smallest usable", "1s", time.Second, false},
		{"zero is refused, not read as off", "0s", 0, true},
		{"a negative duration is refused", "-1h", 0, true},
		{"a non-duration is refused, not read as off", "ninety days", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANALYTICS_RETENTION", tc.value)
			got, err := AnalyticsRetention()
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "ANALYTICS_RETENTION") {
					t.Fatalf("ANALYTICS_RETENTION=%q: err = %v, want a refusal naming the key", tc.value, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("ANALYTICS_RETENTION=%q: got %s, %v; want %s", tc.value, got, err, tc.want)
			}
		})
	}
}
