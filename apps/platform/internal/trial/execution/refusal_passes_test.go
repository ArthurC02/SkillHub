package run

import (
	"errors"
	"fmt"
	"testing"

	policy "github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

var (
	quotaUncounted = refusal{reason: "quota_unavailable", err: fmt.Errorf("%w: db down", policy.ErrAllowanceUnavailable)}
	quotaDaily     = refusal{reason: "quota_daily", err: fmt.Errorf("%w: 20 runs a day", policy.ErrQuotaExceeded)}
	quotaWindow    = refusal{reason: "quota_window", err: fmt.Errorf("%w: 30 runs per 30 days", policy.ErrQuotaExceeded)}
)

func TestOnlyRefusalsThatWillPassOnTheirOwnAreReportedAsPassing(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		passes bool
	}{
		{"the workspace is running as many runs as it may", refusal{reason: ReasonWorkspaceConcurrency, err: errors.New("busy")}, true},
		{"the scanner is unavailable", refusal{reason: ReasonScanUnavailable, err: errors.New("scan")}, true},
		{"dispatch is halted", fmt.Errorf("start: %w", ErrDispatchHalted), true},
		{"the scan blocked the package", refusal{reason: ReasonScanBlocked, err: errors.New("blocked")}, false},
		{"the credit balance is too low", refusal{reason: ReasonCreditBalance, err: errors.New("credit")}, false},
		{"the run quota could not be counted", quotaUncounted, true},
		{"the daily run quota is used up", quotaDaily, false},
		{"the windowed run quota is used up", quotaWindow, false},
		{"an uncountable allowance that is not a refusal", policy.ErrAllowanceUnavailable, false},
		{"an error that is not a refusal", errors.New("database down"), false},
		{"no error", nil, false},
	}
	for _, c := range cases {
		if got := RefusalPasses(c.err); got != c.passes {
			t.Errorf("%s: RefusalPasses = %v, want %v", c.name, got, c.passes)
		}
	}
}

func TestOnlyRefusalsTheOwnerCanOutwaitPauseABatch(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		pauses bool
	}{
		{"the credit balance is too low", refusal{reason: ReasonCreditBalance, err: errors.New("credit")}, true},
		{"the daily run quota is used up", quotaDaily, true},
		{"the windowed run quota is used up", quotaWindow, true},
		{"the run quota could not be counted", quotaUncounted, false},
		{"the scan blocked the package", refusal{reason: ReasonScanBlocked, err: errors.New("blocked")}, false},
		{"an exhausted quota that is not a refusal", policy.ErrQuotaExceeded, false},
		{"no error", nil, false},
	}
	for _, c := range cases {
		if got := RefusalPausesBatch(c.err); got != c.pauses {
			t.Errorf("%s: RefusalPausesBatch = %v, want %v", c.name, got, c.pauses)
		}
	}
}
