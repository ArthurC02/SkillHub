package dockerdrv_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/ArthurC02/skillhub/services/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/services/sandbox/internal/sandbox"
)

// These tests open real containers, which is the only way to check an isolation
// setting: a unit test can assert that the code asked for a read-only rootfs,
// but only a running workload can show that it got one. They skip when no
// daemon is reachable rather than failing, so the suite still runs on a machine
// without Docker — and every container they create carries
// skillhub.sandbox.test=1 and is removed on the way out.
//
// The gVisor half of ADR-015 is not testable here: runsc needs Linux, the
// development machine is Windows, and the escape testing that actually proves
// isolation (SEC-009 / SBX-010) is deployment-time acceptance. What these tests
// cover is the ADR-005 baseline that gVisor sits on top of, not gVisor itself.
const testLabel = "skillhub.sandbox.test"

// testImage only has to run a shell. The real Runtime Image (SBX-002) is
// hundreds of megabytes of Node and is built and scanned by its own pipeline;
// pulling it here would make every test run wait on that for no extra coverage,
// since none of these assertions are about its contents.
func testImage() string {
	if v := os.Getenv("SKILLHUB_SANDBOX_TEST_IMAGE"); v != "" {
		return v
	}
	return "busybox:1.37"
}

func newDriver(t *testing.T) (*dockerdrv.Driver, *client.Client) {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("no docker daemon reachable: %v", err)
	}

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer pullCancel()
	if _, err := cli.ImageInspect(pullCtx, testImage()); err != nil {
		rc, err := cli.ImagePull(pullCtx, testImage(), image.PullOptions{})
		if err != nil {
			t.Skipf("cannot pull %s: %v", testImage(), err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}

	d, err := dockerdrv.New(dockerdrv.Config{
		Image:       testImage(),
		Network:     "none",
		UID:         65532,
		GID:         65532,
		AllowDevCmd: true,
		ExtraLabels: map[string]string{testLabel: "1"},
	})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	t.Cleanup(func() { _ = cli.Close() })
	return d, cli
}

func handle(t *testing.T) string {
	t.Helper()
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "test" + hex.EncodeToString(b[:])
}

func testRequest(script string) sandbox.RunRequest {
	lim := sandbox.DefaultLimits
	lim.MemoryBytes = 256 << 20
	lim.DiskBytes = 64 << 20
	lim.MaxPIDs = 32
	lim.VCPU = 1
	lim.WallClockSoftSeconds = 30
	lim.WallClockHardSeconds = 60
	return sandbox.RunRequest{
		RunID:          "11111111-1111-1111-1111-111111111111",
		RunAttemptID:   "22222222-2222-2222-2222-222222222222",
		Attempt:        1,
		WorkspaceID:    "33333333-3333-3333-3333-333333333333",
		TestCase:       sandbox.TestCaseSnapshotRef{UserPrompt: "isolation probe"},
		Runtime:        sandbox.RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "0.3.233", AgentIntegration: "in_sandbox_sdk"},
		ResourceLimits: lim,
		Egress:         sandbox.EgressPolicy{Mode: "default_deny"},
		Trace:          sandbox.TracePolicy{Level: "standard"},
		Extensions:     map[string]any{"dev_cmd": []any{"sh", "-c", script}},
	}
}

// startProbe runs one throwaway sandbox to completion and returns what it saw.
func startProbe(t *testing.T, d *dockerdrv.Driver, req sandbox.RunRequest) (string, sandbox.Outcome) {
	t.Helper()
	id := handle(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if err := d.Remove(context.Background(), id); err != nil {
			t.Errorf("cleanup of %s failed: %v", id, err)
		}
	})
	if err := d.Start(ctx, id, req); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out, err := d.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	return id, out
}

// The SEC-002 computing-isolation checks, asked of a live sandbox rather than
// of the code that configured it: C-02 non-root and no-new-privileges, C-03
// unprivileged, C-05 no management socket, C-06 read-only base with a writable
// scratch path, C-07 no host paths, C-08 no capabilities, and N-01 no egress.
func TestLiveSandboxMeetsTheIsolationBaseline(t *testing.T) {
	d, cli := newDriver(t)
	script := strings.Join([]string{
		`echo "uid=$(id -u)"`,
		`touch /baseline-probe 2>/dev/null && echo "rootfs=writable" || echo "rootfs=readonly"`,
		`touch /work/probe 2>/dev/null && echo "work=writable" || echo "work=readonly"`,
		`touch /out/probe 2>/dev/null && echo "out=writable" || echo "out=readonly"`,
		`test -e /var/run/docker.sock && echo "docker-socket=present" || echo "docker-socket=absent"`,
		`ip -o addr 2>/dev/null | grep -qv " lo " && echo "net=up" || echo "net=isolated"`,
	}, "\n")

	id, out := startProbe(t, d, testRequest(script))
	for _, want := range []string{
		"uid=65532",            // C-02
		"rootfs=readonly",      // C-06
		"work=writable",        // C-01, the run's own scratch space
		"out=writable",         // C-01
		"docker-socket=absent", // C-05
		"net=isolated",         // N-01
	} {
		if !strings.Contains(out.Output, want) {
			t.Errorf("probe output missing %q; got:\n%s", want, out.Output)
		}
	}
	if out.ExitCode != 0 {
		t.Errorf("probe exited %d", out.ExitCode)
	}

	// The declarative half of the same baseline (threat model 5.6 calls these
	// configuration assertions, not penetration tests).
	insp, err := cli.ContainerInspect(context.Background(), "skillhub-run-"+id)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	hc := insp.HostConfig
	if insp.Config.User != "65532:65532" {
		t.Errorf("User = %q, want a non-root uid:gid", insp.Config.User)
	}
	if !hc.ReadonlyRootfs {
		t.Error("ReadonlyRootfs is off (C-06)")
	}
	if hc.Privileged {
		t.Error("Privileged is on (C-03)")
	}
	if !slices.Contains([]string(hc.CapDrop), "ALL") {
		t.Errorf("CapDrop = %v, want ALL (C-08)", hc.CapDrop)
	}
	if !slices.Contains(hc.SecurityOpt, "no-new-privileges:true") {
		t.Errorf("SecurityOpt = %v, want no-new-privileges (C-02)", hc.SecurityOpt)
	}
	if string(hc.NetworkMode) != "none" {
		t.Errorf("NetworkMode = %q, want none (N-01 dev baseline)", hc.NetworkMode)
	}
	// C-05 and C-07 in one assertion: nothing from the host is mounted at all,
	// so there is no socket and no sensitive path to enumerate.
	if len(hc.Binds) != 0 || len(hc.Mounts) != 0 || len(insp.Mounts) != 0 {
		t.Errorf("host mounts present: binds=%v mounts=%v (C-05, C-07)", hc.Binds, hc.Mounts)
	}
	// C-04: no namespace is shared with the host or with another container.
	// "private" is the daemon's own default and is exactly what we want; what
	// must never appear is "host" or a container: reference.
	for name, mode := range map[string]string{
		"pid": string(hc.PidMode), "ipc": string(hc.IpcMode),
		"uts": string(hc.UTSMode), "network": string(hc.NetworkMode),
	} {
		if mode == "host" || strings.HasPrefix(mode, "container:") {
			t.Errorf("%s namespace = %q, want private (C-04)", name, mode)
		}
	}
	for _, path := range []string{"/work", "/out", "/tmp"} {
		if _, ok := hc.Tmpfs[path]; !ok {
			t.Errorf("no tmpfs at %s: the run has no bounded scratch space (C-01, C-12)", path)
		}
	}
	// C-10, C-11, C-13, C-14.
	lim := testRequest("").ResourceLimits
	if hc.Memory != lim.MemoryBytes {
		t.Errorf("Memory = %d, want %d", hc.Memory, lim.MemoryBytes)
	}
	if hc.MemorySwap != lim.MemoryBytes {
		t.Errorf("MemorySwap = %d, want the memory limit: swap would lift the ceiling", hc.MemorySwap)
	}
	if hc.NanoCPUs != int64(lim.VCPU*1e9) {
		t.Errorf("NanoCPUs = %d, want %d", hc.NanoCPUs, int64(lim.VCPU*1e9))
	}
	if hc.PidsLimit == nil || *hc.PidsLimit != lim.MaxPIDs {
		t.Errorf("PidsLimit = %v, want %d", hc.PidsLimit, lim.MaxPIDs)
	}
	var nofile bool
	for _, u := range hc.Ulimits {
		if u.Name == "nofile" && u.Hard == lim.MaxOpenFiles {
			nofile = true
		}
	}
	if !nofile {
		t.Errorf("Ulimits = %v, want nofile at %d (C-14)", hc.Ulimits, lim.MaxOpenFiles)
	}
}

// C-13, the fork bomb case: the limit has to be enforced by the runtime, not
// merely requested.
func TestPidsLimitStopsAForkBomb(t *testing.T) {
	d, _ := newDriver(t)
	req := testRequest(`i=0; while [ $i -lt 200 ]; do sleep 20 & i=$((i+1)); done; echo "spawned"`)
	req.ResourceLimits.MaxPIDs = 16

	_, out := startProbe(t, d, req)
	if !strings.Contains(strings.ToLower(out.Output), "fork") {
		t.Errorf("200 processes under a 16 pid limit produced no fork failure; output:\n%s", out.Output)
	}
}

// C-15 end to end: the soft wall clock stops the workload, the attempt lands as
// failed with result timed_out (RUN-004), and the sandbox is gone after DELETE.
func TestWallClockStopsALiveSandboxAndDestroyReleasesIt(t *testing.T) {
	d, cli := newDriver(t)
	m := sandbox.NewManager(d, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		Slots:          2,
	}, slog.New(slog.DiscardHandler))

	req := testRequest("sleep 300")
	req.ResourceLimits.WallClockSoftSeconds = 2
	req.ResourceLimits.WallClockHardSeconds = 5

	run, created, err := m.Create(context.Background(), req)
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	t.Cleanup(func() { _ = m.Destroy(context.Background(), run.ProviderRunID) })

	var final sandbox.ProviderRun
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		final, err = m.Get(run.ProviderRunID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if final.State.Terminal() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final.State != sandbox.StateFailed {
		t.Fatalf("state = %s, want failed after the wall clock", final.State)
	}
	if final.Result == nil || final.Result.Status != sandbox.ResultTimedOut {
		t.Fatalf("result = %+v, want status timed_out", final.Result)
	}

	// SBX-009: destroy releases the sandbox, and repeating it is safe.
	for i := range 2 {
		if err := m.Destroy(context.Background(), run.ProviderRunID); err != nil {
			t.Fatalf("destroy #%d: %v", i+1, err)
		}
	}
	if _, err := cli.ContainerInspect(context.Background(), "skillhub-run-"+run.ProviderRunID); !cerrdefs.IsNotFound(err) {
		t.Errorf("container still exists after destroy: %v", err)
	}
}

// Adopt is what a restarted provider has instead of a database: the labels on
// the container it left running.
func TestAdoptRebuildsRunsFromContainerLabels(t *testing.T) {
	d, _ := newDriver(t)
	req := testRequest("sleep 30")
	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })
	if err := d.Start(context.Background(), id, req); err != nil {
		t.Fatalf("start: %v", err)
	}

	found, err := d.Adopt(context.Background())
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	i := slices.IndexFunc(found, func(a sandbox.Adopted) bool { return a.ProviderRunID == id })
	if i < 0 {
		t.Fatalf("adopt did not find the running sandbox %s: %+v", id, found)
	}
	got := found[i]
	if got.RunID != req.RunID || got.Attempt != req.Attempt || !got.Running {
		t.Errorf("adopted = %+v, want the dispatched run still running", got)
	}
	if got.RequestHash != sandbox.HashRequest(req) {
		t.Error("request hash did not survive the restart: a re-sent dispatch would 409")
	}
}
