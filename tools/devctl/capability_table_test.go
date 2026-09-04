package main

import "testing"

// 05 R-36's debt marker: a ledger bucket reason starting with ⛔ names
// variables that gate something but no capability says what. The bucket
// itself is meant to shrink to nothing (closed 2026-09-05) — this test keeps
// it that way: any future ⛔ bucket with variables in it fails CI, the same
// ratchet capability_table.go's own comment describes for db/query-owners.yaml's
// `allow:` list.
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
