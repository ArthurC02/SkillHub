package main

import "testing"

func TestCapabilityLedgerHasNoUnexplainedVariables(t *testing.T) {
	t.Parallel()
	for _, bucket := range capabilityLedger {
		if len(bucket.vars) == 0 {
			continue
		}
		if len(bucket.reason) > 0 && []rune(bucket.reason)[0] == '⛔' {
			t.Errorf("ledger bucket %q still has unexplained variables: %v", bucket.reason, bucket.vars)
		}
	}
}
