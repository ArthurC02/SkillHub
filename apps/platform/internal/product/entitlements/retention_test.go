package policy

import (
	"errors"
	"testing"
	"time"
)

func TestRetentionFailsClosedWithoutAValue(t *testing.T) {
	for _, r := range []DownloadRetention{0, -1} {
		if _, err := r.Period(); !errors.Is(err, ErrRetentionNotConfigured) {
			t.Errorf("DownloadRetention(%d).Period() error = %v, want ErrRetentionNotConfigured", r, err)
		}
	}
	got, err := DownloadRetention(90 * 24 * time.Hour).Period()
	if err != nil || got != 90*24*time.Hour {
		t.Errorf("Period() = (%v, %v), want (2160h0m0s, nil)", got, err)
	}
}
