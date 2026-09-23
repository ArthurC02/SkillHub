//go:build linux

package localdrv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func requireEnforcement(t *testing.T, what string, got bool) {
	t.Helper()
	if got {
		return
	}
	if os.Getenv("SKILLHUB_REQUIRE_CGROUP") == "1" {
		t.Fatalf("SKILLHUB_REQUIRE_CGROUP=1 but this driver cannot enforce %s here; "+
			"this run would have skipped every ceiling test and still reported success", what)
	}
	t.Skipf("no delegated cgroup v2 subtree with the %s controller; skipping", what)
}

func runHog(t *testing.T, script string, lim sandbox.ResourceLimits) (sandbox.Outcome, *Driver, string) {
	t.Helper()
	nodeBin := requireNode(t)
	base := t.TempDir()
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, script), BaseDir: base})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	id := "ceiling-" + strings.TrimSuffix(script, ".mjs")
	req := minimalRequest(id)
	req.ResourceLimits = lim
	if err := d.Start(ctx, id, req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	outcome, err := d.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return outcome, d, id
}

func TestTheMemoryCeilingStopsAWorkloadThatOutgrowsIt(t *testing.T) {
	requireEnforcement(t, "memory", resourceEnforcement().Memory)

	outcome, _, _ := runHog(t, "memory-hog.mjs", sandbox.ResourceLimits{
		MemoryBytes:       192 << 20,
		ArtifactFileBytes: 10 << 20,
	})

	if !strings.Contains(outcome.Output, "the hog is awake") {
		t.Fatalf("the workload never ran, so nothing was under a ceiling: %s", outcome.Output)
	}
	if strings.Contains(outcome.Output, "unhindered") {
		t.Fatalf("the workload allocated past its ceiling: %s", outcome.Output)
	}
	if outcome.ExitCode == 0 {
		t.Fatalf("a workload killed at its memory ceiling reported success (output: %s)", outcome.Output)
	}
}

func TestTheProcessCeilingRefusesTheChildrenPastIt(t *testing.T) {
	requireEnforcement(t, "pids", resourceEnforcement().Processes)

	outcome, _, _ := runHog(t, "fork-storm.mjs", sandbox.ResourceLimits{
		MemoryBytes:       256 << 20,
		MaxPIDs:           40,
		ArtifactFileBytes: 10 << 20,
	})

	if !strings.Contains(outcome.Output, "the storm:") {
		t.Fatalf("the storm never reported back: %s", outcome.Output)
	}
	if strings.Contains(outcome.Output, "refused=0") {
		t.Fatalf("every child was allowed past a ceiling of 40: %s", outcome.Output)
	}
}

func TestACeilingLeavesNoCgroupBehind(t *testing.T) {
	requireEnforcement(t, "pids", resourceEnforcement().Processes)

	parent, err := runParent()
	if err != nil {
		t.Fatalf("runParent: %v", err)
	}
	before := runCgroups(t, parent)

	_, d, id := runHog(t, "fork-storm.mjs", sandbox.ResourceLimits{
		MemoryBytes:       256 << 20,
		MaxPIDs:           40,
		ArtifactFileBytes: 10 << 20,
	})
	if err := d.Remove(context.Background(), id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	for range 100 {
		if len(runCgroups(t, parent)) <= len(before) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("cgroups left behind: before=%v after=%v", before, runCgroups(t, parent))
}

func runCgroups(t *testing.T, parent string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read %s: %v", parent, err)
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "run-") {
			found = append(found, filepath.Join(parent, e.Name()))
		}
	}
	return found
}

func TestAnUnenforceableCeilingDoesNotFailTheRun(t *testing.T) {
	tree := newProcessTree().(*pgroupTree)
	if err := tree.attach(os.Getpid(), treeLimits{}); err != nil {
		t.Fatalf("a run with no ceilings to enforce was refused: %v", err)
	}
	if err := tree.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}
