package policy

import (
	"errors"
	"testing"
	"time"
)

// The fail-closed half of GOV-RETENTION-001: no ratified period, no artifact.
// There is no default and there must not be one — PDM-006's 90 days is a
// proposal, and a default would apply it without anyone having ratified it.
// packaging/retention_test.go holds the other half: that Create actually asks.
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
