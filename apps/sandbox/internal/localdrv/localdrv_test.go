package localdrv

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// requireNode skips the test on a machine with no node runtime, rather than
// failing it: this package's own logic does not need node to compile or to be
// exercised for its host-directory and marker-file bookkeeping, but the
// end-to-end tests below spawn a real process through it.
//
// Except where the environment says node is supposed to be there. Almost every
// test in this file goes through here, so on a machine with no node the package
// reports ok having asserted nothing at all — the same shape the platform's own
// SKILLHUB_REQUIRE_DB guard exists to refuse, and the same variable CI uses to
// insist the Docker tests really ran (SKILLHUB_REQUIRE_DOCKER).
func requireNode(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("SKILLHUB_REQUIRE_DOCKER") == "1" {
			t.Fatalf("SKILLHUB_REQUIRE_DOCKER=1 but node is not on PATH (%v); this run would have "+
				"skipped every localdrv end-to-end test and still reported success", err)
		}
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
//
// It checks two grandchildren, because the platforms differ on exactly one of
// them. The ordinary one must die everywhere. The detached one is checked
// against Reaping().Detached rather than against a wish: where the driver says
// it cannot reach such a process, this asserts it really cannot — so a platform
// that starts reaping it turns this red and forces the declaration to be
// corrected, instead of the two quietly drifting apart. That is not a
// hypothetical: tree_unix.go had the setsid caveat written down correctly while
// this test asserted the opposite, and Linux CI is what said so.
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
	ordinary := filepath.Join(outDir, "ordinary.log")
	detached := filepath.Join(outDir, "detached.log")

	// Wait for both grandchildren to prove they are alive at all, so a failure
	// below means reaping did not work rather than the fixture never having
	// started.
	deadline := time.Now().Add(10 * time.Second)
	for fileSize(t, ordinary) == 0 || fileSize(t, detached) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild heartbeats never started (ordinary=%d detached=%d bytes)",
				fileSize(t, ordinary), fileSize(t, detached))
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
	ordinaryAtStop, detachedAtStop := fileSize(t, ordinary), fileSize(t, detached)
	time.Sleep(700 * time.Millisecond)
	ordinaryAfter, detachedAfter := fileSize(t, ordinary), fileSize(t, detached)

	// The ordinary grandchild must be gone on every platform. This is the
	// assertion that still has teeth where Reaping().Detached is false: without
	// it, a terminate() that did nothing at all would stay green there.
	if ordinaryAfter > ordinaryAtStop {
		t.Fatalf("ordinary grandchild survived Stop: heartbeat grew from %d to %d bytes after the "+
			"driver reported the sandbox stopped — only the direct child was reaped, not the tree "+
			"(report §2's leaked=1 case)", ordinaryAtStop, ordinaryAfter)
	}

	grew := detachedAfter > detachedAtStop
	switch {
	case d.Reaping().Detached && grew:
		t.Fatalf("detached grandchild survived Stop: heartbeat grew from %d to %d bytes, but this "+
			"platform declares Reaping().Detached — the job object no longer holds a process that "+
			"opted out of Node's own cleanup", detachedAtStop, detachedAfter)
	case !d.Reaping().Detached && !grew:
		t.Fatalf("detached grandchild was reaped (heartbeat stopped at %d bytes) on a platform that "+
			"declares it cannot reach one — 02:PORT-010 requires the declaration to reflect what was "+
			"detected, so fix Reaping() rather than this assertion", detachedAtStop)
	}
	if grew {
		// A declared miss, not a leak this test gets to ignore: nothing else
		// will end it, and on Windows a survivor holds the TempDir open and
		// breaks t.Cleanup's own removal.
		killByPIDFile(t, detached+".pid")
	}

	if err := d.Remove(ctx, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// killByPIDFile ends a process this test deliberately left running because the
// driver declares it cannot reach it. Reading the pid from the fixture's own
// file is the only handle available: the process left its parent's group, so
// there is nothing else that still knows about it.
func killByPIDFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("declared-miss cleanup: reading %s: %v", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("declared-miss cleanup: %s does not hold a pid: %v", path, err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("declared-miss cleanup: finding pid %d: %v", pid, err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("declared-miss cleanup: killing pid %d: %v", pid, err)
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

// TestWorkloadEnvironmentIsAnAllowlist is 04 稽核 D2's assertion, made from the
// workload's side of the fence: clean mode's sandboxd is started by
// tools/cleanmode/start.mjs with DATABASE_URL and SKILLHUB_SANDBOX_TOKEN in its
// own environment, and env() used to start from os.Environ(), so both reached
// the workload — where `env` is one Bash tool call away from a trace event that
// no mask pattern knows those two names to redact (iron rule 11).
//
// It asserts on what the child process can actually read, not on the []string
// env() returns: the slice would only prove intent, and cmd.Env is what decides.
func TestWorkloadEnvironmentIsAnAllowlist(t *testing.T) {
	nodeBin := requireNode(t)
	// Not real credentials — the point is only that a name of this shape does
	// not travel, so the values are literals a grep for a secret will not hit.
	t.Setenv("DATABASE_URL", "postgres://not-a-real-dsn/skillhub")
	t.Setenv("SKILLHUB_SANDBOX_TOKEN", "not-a-real-token")
	t.Setenv("FOO_SECRET", "not-a-real-secret")

	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "envdump.mjs"), BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "envdump-1"
	if err := d.Start(ctx, id, minimalRequest(id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	deadline := time.Now().Add(15 * time.Second)
	for {
		done, err := d.WorkloadDone(ctx, id)
		if err != nil {
			t.Fatalf("WorkloadDone: %v", err)
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("envdump workload never signalled WorkloadDone")
		}
		time.Sleep(50 * time.Millisecond)
	}

	raw, err := d.ReadArtifacts(ctx, id)
	if err != nil {
		t.Fatalf("ReadArtifacts: %v", err)
	}
	body := artifactNamed(t, raw, "env.json")
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("env dump is not JSON: %v (%s)", err, body)
	}

	for _, forbidden := range []string{"DATABASE_URL", "SKILLHUB_SANDBOX_TOKEN", "FOO_SECRET"} {
		if v, ok := got[forbidden]; ok {
			t.Errorf("%s reached the workload (=%q): env() must build from an allowlist, not from os.Environ()", forbidden, v)
		}
	}
	// The allowlist has to actually let node run, which is the half a denylist
	// gets for free and this one has to earn: a workload that cannot find its
	// own interpreter's dependents never gets far enough to fail the check above.
	if pathOf(got) == "" {
		t.Errorf("PATH did not reach the workload; env() = %v", got)
	}
	if got["SKILLHUB_RUN_ID"] != id {
		t.Errorf("SKILLHUB_RUN_ID = %q, want %q: the driver's own keys must still be set", got["SKILLHUB_RUN_ID"], id)
	}
}

// pathOf reads PATH under either spelling: Windows reports it as `Path`, and
// this driver passes the host's own name through unchanged.
func pathOf(env map[string]string) string {
	for k, v := range env {
		if strings.EqualFold(k, "PATH") {
			return v
		}
	}
	return ""
}

// artifactNamed pulls one file out of the tar ReadArtifacts returns.
func artifactNamed(t *testing.T, raw []byte, name string) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("artifact %q not found in the collected tar", name)
		}
		if err != nil {
			t.Fatalf("reading artifact tar: %v", err)
		}
		if hdr.Name == name {
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("reading artifact %q: %v", name, err)
			}
			return body
		}
	}
}

// TestReadArtifactsSkipsNonRegularEntries: one unusable entry must cost the run
// that entry, not every artifact it produced.
//
// Before this, a symlink in the artifacts directory made tar.FileInfoHeader
// write a header whose Size was the link target's *string length* while the
// copy that followed it read the pointed-at file's contents, so tar.Writer
// returned ErrWriteTooLong and ReadArtifacts failed as a whole. The manager
// logs that at Warn and returns a nil manifest, and the run still reports
// success — so the user sees a finished run with no output and no reason, with
// the explanation only in a node log they cannot read. dockerdrv skips the same
// entry and collects the rest; two drivers behind one interface must not
// disagree about that.
func TestReadArtifactsSkipsNonRegularEntries(t *testing.T) {
	nodeBin := requireNode(t)
	base := t.TempDir()
	d, err := New(Config{NodeBin: nodeBin, RunnerScript: testdataScript(t, "workload.mjs"), BaseDir: base})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "nonregular-1"
	if err := d.Start(ctx, id, minimalRequest(id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	deadline := time.Now().Add(15 * time.Second)
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

	// workload.mjs has already written the one regular file; add the entry a
	// hostile or careless workload could have written beside it.
	_, outDir := d.paths(id)
	makeNonRegularEntry(t, outDir)

	raw, err := d.ReadArtifacts(ctx, id)
	if err != nil {
		t.Fatalf("ReadArtifacts failed on a directory holding one non-regular entry: %v; "+
			"the entry must be skipped, not turn the whole collection into nothing", err)
	}
	var names []string
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading artifact tar: %v", err)
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 1 || names[0] != "result.txt" {
		t.Fatalf("collected %v, want exactly [result.txt]", names)
	}
}

// makeNonRegularEntry puts one entry that is neither a regular file nor a
// directory into the run's artifacts directory, by whatever means the platform
// allows an unprivileged process.
//
// A POSIX symlink is the direct case. Windows refuses one without
// SeCreateSymbolicLinkPrivilege, which an ordinary account does not have — but
// a directory junction needs no privilege and Go's WalkDir reports it as
// irregular (neither IsDir nor IsRegular), which is the same branch. That
// matters here rather than being a curiosity: Windows is the platform this
// driver was written for, so this assertion must have teeth there.
func makeNonRegularEntry(t *testing.T, outDir string) {
	t.Helper()
	dir := artifactDir(outDir)
	link := filepath.Join(dir, "link")
	target := filepath.Join(outDir, "target")
	if err := os.WriteFile(target, []byte("pointed at, not collected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err == nil {
		return
	}
	if runtime.GOOS != "windows" {
		t.Skipf("cannot create a symlink in %s and this is not Windows", dir)
	}
	junctionTarget := filepath.Join(outDir, "junction-target")
	if err := os.MkdirAll(junctionTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, junctionTarget).CombinedOutput()
	if err != nil {
		t.Skipf("no symlink privilege and mklink /J failed (%v): %s", err, out)
	}
}
