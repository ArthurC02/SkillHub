package dockerdrv_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const testLabel = "skillhub.sandbox.test"

func testImage() string {
	if v := os.Getenv("SKILLHUB_SANDBOX_TEST_IMAGE"); v != "" {
		return v
	}
	return "busybox:1.37@sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0"
}

func requireDocker() bool { return os.Getenv("SKILLHUB_REQUIRE_DOCKER") == "1" }

func skipOrFail(t *testing.T, format string, args ...any) {
	t.Helper()
	if requireDocker() {
		t.Fatalf("SKILLHUB_REQUIRE_DOCKER=1 but "+format+
			"; this run would have skipped every Docker test and still reported success", args...)
	}
	t.Skipf(format, args...)
}

func testRuntime() string { return os.Getenv("SKILLHUB_SANDBOX_TEST_RUNTIME") }

func dockerClient(t *testing.T) *client.Client {
	t.Helper()
	cli, err := client.New(client.FromEnv)
	if err != nil {
		skipOrFail(t, "no docker client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		skipOrFail(t, "no docker daemon reachable: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func newDriver(t *testing.T) (*dockerdrv.Driver, *client.Client) {
	t.Helper()
	cli := dockerClient(t)

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer pullCancel()
	if _, err := cli.ImageInspect(pullCtx, testImage()); err != nil {
		rc, err := cli.ImagePull(pullCtx, testImage(), client.ImagePullOptions{})
		if err != nil {
			skipOrFail(t, "cannot pull %s: %v", testImage(), err)
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
		Runtime:     testRuntime(),
		ExtraLabels: map[string]string{testLabel: "1"},
	})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
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
		"uid=65532",
		"rootfs=readonly",
		"work=writable",
		"out=writable",
		"docker-socket=absent",
		"net=isolated",
	} {
		if !strings.Contains(out.Output, want) {
			t.Errorf("probe output missing %q; got:\n%s", want, out.Output)
		}
	}
	if out.ExitCode != 0 {
		t.Errorf("probe exited %d", out.ExitCode)
	}

	inspected, err := cli.ContainerInspect(context.Background(), "skillhub-run-"+id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	insp := inspected.Container
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

	if len(hc.Binds) != 0 || len(hc.Mounts) != 0 || len(insp.Mounts) != 0 {
		t.Errorf("host mounts present: binds=%v mounts=%v (C-05, C-07)", hc.Binds, hc.Mounts)
	}

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

func TestPidsLimitStopsAForkBomb(t *testing.T) {
	d, _ := newDriver(t)
	const (
		pidCeiling    = 128
		spawnAttempts = 400
	)

	alive := testRequest(`echo alive`)
	alive.ResourceLimits.MaxPIDs = pidCeiling
	if _, out := startProbe(t, d, alive); !strings.Contains(out.Output, "alive") {
		t.Fatalf("a workload that does nothing but echo produced no output under a %d pid ceiling, "+
			"so this runtime needs more headroom than that before C-13 can be measured at all. output:\n%s",
			pidCeiling, out.Output)
	}

	req := testRequest(fmt.Sprintf(
		`i=0; while [ $i -lt %d ]; do sleep 20 & i=$((i+1)); done; echo "spawned"`, spawnAttempts))
	req.ResourceLimits.MaxPIDs = pidCeiling

	_, out := startProbe(t, d, req)
	if out.ExitCode == 0 && strings.Contains(out.Output, "spawned") {
		t.Errorf("a workload asking for %d processes under a %d pid ceiling ran to completion "+
			"(exit %d), and a trivial workload ran under the same ceiling: the limit is not "+
			"reaching guest tasks on this runtime. output:\n%s",
			spawnAttempts, pidCeiling, out.ExitCode, out.Output)
	}
}

func TestMemoryCeilingStopsAWorkloadThatExceedsIt(t *testing.T) {
	d, _ := newDriver(t)

	req := testRequest(`dd if=/dev/zero of=/work/ok bs=1M count=64 2>/dev/null; echo "completed rc=$?"`)
	req.ResourceLimits.MemoryBytes = 256 << 20
	req.ResourceLimits.DiskBytes = 1 << 30
	if _, out := startProbe(t, d, req); !strings.Contains(out.Output, "completed rc=0") {
		t.Fatalf("a workload writing 64 MiB under a 256 MiB memory ceiling did not complete, "+
			"so this runtime needs more headroom than that before C-11 can be measured at all. output:\n%s",
			out.Output)
	}

	req.Extensions = map[string]any{"dev_cmd": []any{"sh", "-c",
		`dd if=/dev/zero of=/work/hog bs=1M count=512 2>/dev/null; echo "completed rc=$?"`}}
	_, out := startProbe(t, d, req)
	if strings.Contains(out.Output, "completed rc=0") {
		t.Errorf("a workload wrote 512 MiB into a sandbox with a 256 MiB memory ceiling and reported "+
			"success: the ceiling is not reaching guest tasks on this runtime (C-11). output:\n%s",
			out.Output)
	}
}

func TestScratchQuotaRefusesAWriteWithoutKillingTheRun(t *testing.T) {
	d, _ := newDriver(t)

	const workBytes = 48 << 20

	req := testRequest(`dd if=/dev/zero of=/work/small bs=1M count=8 2>/dev/null; echo "small rc=$?"`)
	if _, out := startProbe(t, d, req); !strings.Contains(out.Output, "small rc=0") {
		t.Fatalf("an 8 MiB write into a %d MiB scratch space failed, so the fixture is wrong and "+
			"C-12 cannot be measured. output:\n%s", workBytes>>20, out.Output)
	}

	req.Extensions = map[string]any{"dev_cmd": []any{"sh", "-c",
		`dd if=/dev/zero of=/work/fill bs=1M count=128 2>/dev/null; echo "fill rc=$?"; echo "size=$(wc -c < /work/fill)"`}}
	_, out := startProbe(t, d, req)
	if strings.Contains(out.Output, "fill rc=0") {
		t.Errorf("a 128 MiB write into a %d MiB scratch space reported success: the quota is not "+
			"enforced on this runtime (C-12). output:\n%s", workBytes>>20, out.Output)
	}
	var size int64
	if _, err := fmt.Sscanf(sizeLine(out.Output), "size=%d", &size); err != nil {
		t.Fatalf("the workload did not report the file size, so nothing was measured (C-12). output:\n%s", out.Output)
	}
	if size > workBytes {
		t.Errorf("the file reached %d bytes in a %d byte scratch space: the quota did not hold (C-12)", size, workBytes)
	}
}

func TestOpenFileCeilingReachesGuestTasks(t *testing.T) {
	d, _ := newDriver(t)
	const ceiling = 64

	alive := testRequest(`echo alive`)
	alive.ResourceLimits.MaxOpenFiles = ceiling
	if _, out := startProbe(t, d, alive); !strings.Contains(out.Output, "alive") {
		t.Fatalf("a workload that does nothing but echo produced no output under a %d fd ceiling, "+
			"so this runtime needs more headroom than that before C-14 can be measured at all. output:\n%s",
			ceiling, out.Output)
	}

	req := testRequest(`ulimit -n; i=3; while [ $i -lt 200 ]; do eval "exec $i<>/work/fdprobe"; i=$((i+1)); done; echo "opened-all-200"`)
	req.ResourceLimits.MaxOpenFiles = ceiling
	_, out := startProbe(t, d, req)
	if !strings.Contains(out.Output, strconv.Itoa(ceiling)) {
		t.Errorf("ulimit -n inside the sandbox did not report %d: the rlimit did not reach the guest "+
			"task (C-14). output:\n%s", ceiling, out.Output)
	}
	if strings.Contains(out.Output, "opened-all-200") {
		t.Errorf("a workload opened 200 files under a %d fd ceiling: the limit is not enforced on this "+
			"runtime (C-14). output:\n%s", ceiling, out.Output)
	}
}

func sizeLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "size=") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

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

	for i := range 2 {
		if err := m.Destroy(context.Background(), run.ProviderRunID); err != nil {
			t.Fatalf("destroy #%d: %v", i+1, err)
		}
	}
	if _, err := cli.ContainerInspect(context.Background(), "skillhub-run-"+run.ProviderRunID, client.ContainerInspectOptions{}); !cerrdefs.IsNotFound(err) {
		t.Errorf("container still exists after destroy: %v", err)
	}
}

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

func TestRequestedRuntimeIsTheOneTheContainerGot(t *testing.T) {
	want := testRuntime()
	if want == "" {
		t.Skip("no runtime requested; the daemon default is what the other tests exercise")
	}
	d, cli := newDriver(t)
	ctx := context.Background()
	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	if err := d.Start(ctx, id, testRequest("sleep 30")); err != nil {
		t.Fatalf("starting a container on runtime %q failed: %v", want, err)
	}
	info, err := cli.ContainerInspect(ctx, "skillhub-run-"+id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Container.HostConfig.Runtime != want {
		t.Fatalf("asked the daemon for runtime %q and got %q", want, info.Container.HostConfig.Runtime)
	}
	if err := d.Stop(ctx, id, time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
}
