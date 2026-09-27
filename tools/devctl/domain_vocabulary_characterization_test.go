package main

import "testing"

func TestTheRunStatusSourceNamesItsFileAndEnum(t *testing.T) {
	t.Parallel()
	if got := runStatusEnum("db/migrations/0001_runs.sql").label; got != "db/migrations/0001_runs.sql (enum run_status)" {
		t.Fatalf("label = %q", got)
	}
}
