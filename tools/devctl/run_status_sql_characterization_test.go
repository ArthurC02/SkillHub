package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnlyAStatusListThatNamesTimedOutIsReadAsTheTerminalSet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "db", "queries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sql := "-- name: ListActive :many\nSELECT id FROM runs WHERE status IN ('queued', 'running');\n"
	if err := os.WriteFile(filepath.Join(dir, "runs.sql"), []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"queued": true, "running": true, "succeeded": true, "timed_out": true}
	terminal := map[string]bool{"succeeded": true, "timed_out": true}
	if problems := runTerminalListProblems(root, known, terminal); len(problems) != 0 {
		t.Fatalf("a list of live statuses was compared with the terminal set: %v", problems)
	}
}
