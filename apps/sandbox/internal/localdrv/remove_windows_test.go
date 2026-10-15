package localdrv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestARunDirectoryThatCannotBeRemovedFailsRemoveUntilItCan(t *testing.T) {
	base := t.TempDir()
	d, err := New(Config{NodeBin: requireNode(t), RunnerScript: testdataScript(t, "heartbeat.mjs"), BaseDir: base})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	const id = "held"
	if err := d.Start(t.Context(), id, minimalRequest(id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	workDir, _ := d.paths(id)
	held, err := os.Create(filepath.Join(workDir, "held-open"))
	if err != nil {
		t.Fatal(err)
	}

	if err := d.Remove(t.Context(), id); err == nil {
		t.Fatal("Remove reported success while the run directory was still on disk")
	}
	if _, err := os.Stat(filepath.Join(base, id)); err != nil {
		t.Fatalf("run directory: %v, want it still present after the failed removal", err)
	}

	_ = held.Close()
	if err := d.Remove(t.Context(), id); err != nil {
		t.Fatalf("Remove after the file was released: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, id)); !os.IsNotExist(err) {
		t.Fatalf("run directory after removal: %v, want it gone", err)
	}
}
