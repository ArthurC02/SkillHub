package main

import (
	"encoding/json"
	"io"
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

func TestGenerationLockMetadataIsWorldReadable(t *testing.T) {
	root := t.TempDir()
	release, err := acquireGenerationLock(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	info, err := os.Stat(filepath.Join(root, ".devctl", "generate.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got&0o044 != 0o044 {
		t.Fatalf("lock mode = %o; want readable metadata", got)
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

func TestCompareTreesIgnoresPythonBytecode(t *testing.T) {
	generated := t.TempDir()
	committed := t.TempDir()
	writeTestFile(t, generated, "models.py", "same")
	writeTestFile(t, committed, "models.py", "same")
	writeTestFile(t, committed, "__pycache__/models.cpython-312.pyc", "transient")
	drift, err := compareTrees(generated, committed)
	if err != nil {
		t.Fatal(err)
	}
	if len(drift) != 0 {
		t.Fatalf("bytecode caused drift: %v", drift)
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

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSyncDriftedFilesWritesChangedAndAddedFilesAndRemovesDeletedOnes(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTestFile(t, source, "changed.go", "new")
	writeTestFile(t, target, "changed.go", "old")
	writeTestFile(t, source, "sub/added.go", "added")
	writeTestFile(t, target, "removed.go", "gone")
	writeTestFile(t, target, "kept.go", "kept")
	if err := syncDriftedFiles(source, target, []string{"changed.go", "sub/added.go", "removed.go"}); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(target, "changed.go")) + readTestFile(t, filepath.Join(target, "sub", "added.go")); got != "newadded" {
		t.Fatalf("synced contents = %q", got)
	}
	if _, err := os.Stat(filepath.Join(target, "removed.go")); !os.IsNotExist(err) {
		t.Fatalf("removed.go remains: %v", err)
	}
	if readTestFile(t, filepath.Join(target, "kept.go")) != "kept" {
		t.Fatal("a file outside the drift changed")
	}
}

func TestInstallingDriftedOutputsInstallsTheRestWhenOneFails(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	writeTestFile(t, good, "a.go", "new")
	pending := []driftedOutput{
		{generationOutput: generationOutput{label: "broken", source: filepath.Join(root, "missing"), target: filepath.Join(root, "broken-target")}, drift: []string{"a.go"}},
		{generationOutput: generationOutput{label: "good", source: good, target: filepath.Join(root, "good-target")}, drift: []string{"a.go"}},
	}
	err := installDriftedOutputs(pending, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "replace broken output") || !strings.Contains(err.Error(), "may hold a file") {
		t.Fatalf("error = %v", err)
	}
	if readTestFile(t, filepath.Join(root, "good-target", "a.go")) != "new" {
		t.Fatal("the second output was not installed")
	}
}

func TestInstallingFallsBackToFileSyncWhenTheDirectoryCannotMove(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	target := filepath.Join(parent, "generated")
	source := filepath.Join(root, "source")
	writeTestFile(t, target, "a.go", "old")
	writeTestFile(t, source, "a.go", "new")
	held, err := os.Open(filepath.Join(target, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	if atomicReplaceDir(source, target) == nil {
		t.Skip("this platform let the held directory move; the fallback is not reached")
	}
	held.Close()
	held, _ = os.Open(filepath.Join(target, "a.go"))
	if err := installOutput(driftedOutput{generationOutput: generationOutput{label: "ts", source: source, target: target}, drift: []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	held.Close()
	if got := readTestFile(t, filepath.Join(target, "a.go")); got != "new" {
		t.Fatalf("a.go = %q", got)
	}
}

func TestPruningRemovesOnlyOldGenerationScratch(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for _, name := range []string{"generate-old", "generate-new", "other-old"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ages := map[string]time.Time{
		"generate-old": now.Add(-scratchMaxAge - time.Hour),
		"other-old":    now.Add(-scratchMaxAge - time.Hour),
		"generate-new": now.Add(-time.Hour),
	}
	for name, at := range ages {
		if err := os.Chtimes(filepath.Join(dir, name), at, at); err != nil {
			t.Fatal(err)
		}
	}
	pruneOldScratch(dir, now)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, entry := range entries {
		left = append(left, entry.Name())
	}
	if got := strings.Join(left, ","); got != "generate-new,other-old" {
		t.Fatalf("left = %q", got)
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
