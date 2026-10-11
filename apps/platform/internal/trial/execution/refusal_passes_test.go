package run

import (
	"errors"
	"fmt"
	"testing"
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
		{"an error that is not a refusal", errors.New("database down"), false},
		{"no error", nil, false},
	}
	for _, c := range cases {
		if got := RefusalPasses(c.err); got != c.passes {
			t.Errorf("%s: RefusalPasses = %v, want %v", c.name, got, c.passes)
		}
	}
}
