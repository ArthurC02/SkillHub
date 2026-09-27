package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnUnreadableGenerationLockIsTreatedAsHeld(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".devctl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "generate.lock"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := acquireGenerationLock(root, time.Now().Add(10*generationLockMaxAge))
	if err == nil {
		release()
		t.Fatal("a lock whose age cannot be read was taken over")
	}
	if !strings.Contains(err.Error(), "generation is already running (not json)") {
		t.Fatalf("err = %v, want it to quote the lock it refused to take over", err)
	}
}
