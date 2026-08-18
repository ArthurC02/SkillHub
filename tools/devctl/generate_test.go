package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerationLockIsExclusive(t *testing.T) {
	root := t.TempDir()
	release, err := acquireGenerationLock(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := acquireGenerationLock(root, time.Now()); err == nil || !strings.Contains(err.Error(), "one writer") {
		t.Fatalf("second lock error = %v", err)
	}
}

func TestGenerationLockReclaimsExpiredFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".devctl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(generationLock{PID: 123, CreatedAt: time.Now().Add(-generationLockMaxAge - time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "generate.lock"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := acquireGenerationLock(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestCompareTreesReportsAddedChangedAndRemovedFiles(t *testing.T) {
	generated := t.TempDir()
	committed := t.TempDir()
	writeTestFile(t, generated, "same.go", "same")
	writeTestFile(t, committed, "same.go", "same")
	writeTestFile(t, generated, "changed.go", "new")
	writeTestFile(t, committed, "changed.go", "old")
	writeTestFile(t, generated, "added.go", "added")
	writeTestFile(t, committed, "removed.go", "removed")
	drift, err := compareTrees(generated, committed)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(drift, ",")
	if got != "added.go,changed.go,removed.go" {
		t.Fatalf("drift = %q", got)
	}
}

func TestAtomicReplaceDir(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	writeTestFile(t, source, "generated.go", "new")
	writeTestFile(t, target, "generated.go", "old")
	writeTestFile(t, target, "obsolete.go", "remove")
	if err := atomicReplaceDir(source, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("generated.go = %q", data)
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.go")); !os.IsNotExist(err) {
		t.Fatalf("obsolete file remains: %v", err)
	}
}

func writeTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
