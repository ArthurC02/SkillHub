package localdrv

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// requireNode skips the test on a machine with no node runtime, rather than
// failing it: this package's own logic does not need node to compile or to be
// exercised for its host-directory and marker-file bookkeeping, but the
// end-to-end tests below spawn a real process through it.
func requireNode(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH; skipping localdrv end-to-end tests")
	}
	return bin
}

// testdataScript resolves a fixture under testdata/ to an absolute path.
// Start sets cmd.Dir to the run's own work directory, and an OS resolves a
// relative executable argument against the *child's* working directory, not
// the test binary's — a relative "testdata/x.mjs" would otherwise silently
// resolve against a different, empty directory and fail to launch at all.
func testdataScript(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func minimalRequest(id string) sandbox.RunRequest {
	return sandbox.RunRequest{
		RunID:        id,
		RunAttemptID: id + "-attempt",
		Attempt:      1,
		WorkspaceID:  "ws-1",
		TestCase:     sandbox.TestCaseSnapshotRef{UserPrompt: "say hello"},
		Runtime:      sandbox.RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "0.3.233"},
		ResourceLimits: sandbox.ResourceLimits{
			MemoryBytes:       256 << 20,
			MaxPIDs:           32,
			ArtifactFileBytes: 10 << 20,
		},
		Trace: sandbox.TracePolicy{Level: "standard"},
	}
}

func TestNewRejectsMissingRunnerScript(t *testing.T) {
	nodeBin := requireNode(t)
	if _, err := New(Config{NodeBin: nodeBin, BaseDir: t.TempDir()}); err == nil {
		t.Fatal("expected New to refuse a Config with no RunnerScript")
	}
}

func TestNewRejectsMissingNode(t *testing.T) {
	if _, err := New(Config{NodeBin: "skillhub-node-that-does-not-exist", RunnerScript: "x.mjs"}); err == nil {
		t.Fatal("expected New to refuse a Config whose node binary cannot be found")
	}
}

func TestHealthy(t *testing.T) {
	nodeBin := requireNode(t)
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "workload.mjs"), BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Healthy(context.Background()) {
		t.Fatal("expected Healthy to be true when node resolves and runs")
	}
}

func TestAdoptReturnsNothing(t *testing.T) {
	// ADR-059 decision 5①. Not a placeholder: this is the whole answer.
	nodeBin := requireNode(t)
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "workload.mjs"), BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Adopt(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("Adopt() = %v, %v; want nil, nil", got, err)
	}
}

func TestRemoveAndStopAreIdempotentOnUnknownID(t *testing.T) {
	nodeBin := requireNode(t)
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "workload.mjs"), BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := d.Remove(ctx, "no-such-run"); err != nil {
		t.Fatalf("Remove on an unknown id should succeed (SBX-009): %v", err)
	}
	if err := d.Stop(ctx, "no-such-run", time.Second); err != nil {
		t.Fatalf("Stop on an unknown id should succeed: %v", err)
	}
}

// TestDriverLifecycle exercises Start, the WorkloadDone/ReleaseWorkload
// collection handshake, ReadTrace, ReadArtifacts, Wait and Remove against a
// workload that speaks the same marker-file protocol run.mjs does, without
// needing the Claude Agent SDK, a network route or an API key.
func TestDriverLifecycle(t *testing.T) {
	nodeBin := requireNode(t)
	base := t.TempDir()
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "workload.mjs"), BaseDir: base})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "lifecycle-1"
	req := minimalRequest(id)

	if err := d.Start(ctx, id, req); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		done, err := d.WorkloadDone(ctx, id)
		if err != nil {
			t.Fatalf("WorkloadDone: %v", err)
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("workload never signalled WorkloadDone")
		}
		time.Sleep(50 * time.Millisecond)
	}

	trace, more, err := d.ReadTrace(ctx, id, 0)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if more {
		t.Fatal("ReadTrace reported more data than the tiny fixture trace could contain")
	}
	if !bytes.Contains(trace, []byte(`"run_id":"`+id+`"`)) {
		t.Fatalf("ReadTrace did not carry the expected event: %s", trace)
	}

	rawArtifacts, err := d.ReadArtifacts(ctx, id)
	if err != nil {
		t.Fatalf("ReadArtifacts: %v", err)
	}
	found := false
	tr := tar.NewReader(bytes.NewReader(rawArtifacts))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading artifact tar: %v", err)
		}
		if hdr.Name == "result.txt" {
			found = true
			body, _ := io.ReadAll(tr)
			if string(body) != "hello from the workload\n" {
				t.Fatalf("unexpected artifact content: %q", body)
			}
		}
	}
	if !found {
		t.Fatal("expected artifact result.txt was not in the tar")
	}

	if err := d.ReleaseWorkload(ctx, id); err != nil {
		t.Fatalf("ReleaseWorkload: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	outcome, err := d.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("outcome.ExitCode = %d, want 0 (output: %s)", outcome.ExitCode, outcome.Output)
	}

	if err := d.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, id)); !os.IsNotExist(err) {
		t.Fatalf("Remove did not clean up the run directory: %v", err)
	}
}

// TestReapsWholeProcessTree is 02:PORT-010's own required check: a
// pgroup/job-based Stop must end a grandchild the workload spawned, not just
// the workload itself. It reproduces the exact shape
// docs/plans/mvp/m6/report-local-driver.md §2 measured — a plain kill of the
// direct child leaves the grandchild running (leaked=1 there); this test fails
// the same way if this driver's tree.terminate ever regresses to that.
func TestReapsWholeProcessTree(t *testing.T) {
	nodeBin := requireNode(t)
	base := t.TempDir()
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "reaper.mjs"), BaseDir: base})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "tree-1"
	if err := d.Start(ctx, id, minimalRequest(id)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, outDir := d.paths(id)
	heartbeat := filepath.Join(outDir, "heartbeat.log")

	// Wait for the grandchild to prove it is alive at all, so a failure below
	// means reaping did not work rather than the fixture never having started.
	deadline := time.Now().Add(10 * time.Second)
	for fileSize(t, heartbeat) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("grandchild heartbeat never started")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := d.Stop(ctx, id, 0); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the kill call's own effect (TerminateJobObject / kill(-pgid)) time
	// to actually land, and any heartbeat write already in flight time to
	// finish, before taking the "at rest" baseline.
	time.Sleep(300 * time.Millisecond)
	atStop := fileSize(t, heartbeat)
	time.Sleep(700 * time.Millisecond)
	afterWait := fileSize(t, heartbeat)

	if afterWait > atStop {
		t.Fatalf("grandchild survived Stop: heartbeat grew from %d to %d bytes after the driver "+
			"reported the sandbox stopped — only the direct child was reaped, not the whole tree "+
			"(report §2's leaked=1 case)", atStop, afterWait)
	}

	if err := d.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
