package mxcdrv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/drivertest"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const gatewayKey = "sk-mxcdrv-test-virtual-key-7f3a"

type harness struct {
	driver  *Driver
	baseDir string
	script  string
	nodeBin string
}

func requireNode(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH; the fake mxc starts a real node workload")
	}
	return bin
}

func testdataScript(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func newHarness(t *testing.T, mxcBin, script string) harness {
	t.Helper()
	nodeBin := requireNode(t)
	h := harness{baseDir: t.TempDir(), script: testdataScript(t, script), nodeBin: nodeBin}
	d, err := New(Config{MXCBin: mxcBin, NodeBin: nodeBin, RunnerScript: h.script, BaseDir: h.baseDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	h.driver = d
	return h
}

func newHandle(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("handle: %v", err)
	}
	return "mxc" + hex.EncodeToString(b[:])
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
		Trace:        sandbox.TracePolicy{Level: "standard"},
		ModelGateway: &sandbox.ModelGatewayGrant{BaseURL: "http://gateway.invalid:4000", VirtualKey: gatewayKey},
	}
}

func (h harness) run(t *testing.T) (string, sandbox.Outcome, error) {
	t.Helper()
	id := newHandle(t)
	t.Cleanup(func() { _ = h.driver.Remove(context.Background(), id) })
	if err := h.driver.Start(context.Background(), id, minimalRequest(id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := h.driver.Wait(ctx, id)
	return id, out, err
}

func (h harness) runDirs(id string) (workDir, outDir string) {
	root := filepath.Join(h.baseDir, id)
	return filepath.Join(root, "work"), filepath.Join(root, "out")
}

func (h harness) policyPath(id string) string {
	return filepath.Join(h.baseDir, id, configName)
}

func (h harness) readPolicy(t *testing.T, id string) (policy, []byte) {
	t.Helper()
	raw, err := os.ReadFile(h.policyPath(id))
	if err != nil {
		t.Fatalf("read the run policy: %v", err)
	}
	var p policy
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode the run policy: %v", err)
	}
	return p, raw
}

func TestTheMXCDriverMeetsTheDriverContract(t *testing.T) {
	fake := fakeMXC(t, fakeWorks)
	drivertest.RunContract(t, drivertest.Subject{
		New: func(t *testing.T) sandbox.Driver {
			return newHarness(t, fake, "exits-zero.mjs").driver
		},
		Handle: newHandle,
		Request: func(t *testing.T) sandbox.RunRequest {
			t.Helper()
			return minimalRequest("contract-run")
		},
	})
}

func TestTheRunPolicyLetsTheWorkloadWriteOnlyItsWorkAndOutputDirectories(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	id, _, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	p, _ := h.readPolicy(t, id)
	workDir, outDir := h.runDirs(id)
	if want := []string{workDir, outDir}; !slices.Equal(p.Filesystem.ReadwritePaths, want) {
		t.Errorf("readwritePaths = %v, want exactly %v", p.Filesystem.ReadwritePaths, want)
	}
	if want := []string{filepath.Dir(h.script)}; !slices.Equal(p.Filesystem.ReadonlyPaths, want) {
		t.Errorf("readonlyPaths = %v, want exactly the runner script's directory %v", p.Filesystem.ReadonlyPaths, want)
	}
}

func TestTheRunPolicyBlocksTheNetworkAndDestroysTheSandboxOnExit(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	id, _, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	p, _ := h.readPolicy(t, id)
	if p.Network.DefaultPolicy != "block" {
		t.Errorf("network.defaultPolicy = %q, want block", p.Network.DefaultPolicy)
	}
	if !p.Lifecycle.DestroyOnExit {
		t.Error("lifecycle.destroyOnExit = false, want true")
	}
}

func TestTheRunPolicyStartsTheRunnerScriptWithNodeInTheWorkDirectory(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	id, _, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	p, _ := h.readPolicy(t, id)
	workDir, _ := h.runDirs(id)
	if p.Process.Cwd != workDir {
		t.Errorf("process.cwd = %q, want %q", p.Process.Cwd, workDir)
	}
	if got, want := splitCommandLine(p.Process.CommandLine), []string{h.nodeBin, h.script}; !slices.Equal(got, want) {
		t.Errorf("process.commandLine splits into %q, want %q", got, want)
	}
}

func TestTheRunPolicyCarriesNoEnvironmentSoTheGatewayKeyNeverReachesDisk(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	id, _, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	_, raw := h.readPolicy(t, id)
	var fields struct {
		Process map[string]json.RawMessage `json:"process"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode the run policy: %v", err)
	}
	if _, ok := fields.Process["env"]; ok {
		t.Errorf("the run policy has a process.env field: %s", raw)
	}
	if strings.Contains(string(raw), gatewayKey) {
		t.Errorf("the gateway key was written into the run policy: %s", raw)
	}
}

func TestTheGatewayKeyReachesTheWorkloadThroughTheMXCProcessEnvironment(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "prints-gateway-key.mjs")
	_, out, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if want := "key=" + gatewayKey; !strings.Contains(out.Output, want) {
		t.Errorf("workload output = %q, want it to contain %q", out.Output, want)
	}
}

func TestTheRunPolicyIsReadableOnlyByTheProviderAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not map file mode bits to access control; the 0600 mode is only observable on POSIX")
	}
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	id, _, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	info, err := os.Stat(h.policyPath(id))
	if err != nil {
		t.Fatalf("stat the run policy: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("run policy mode = %o, want 600", got)
	}
}

func TestAWorkloadThatExitsWith127IsReportedAsAWorkloadExit(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-127.mjs")
	_, out, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait returned %v for a workload that ran and exited 127", err)
	}
	if out.ExitCode != 127 {
		t.Errorf("exit code = %d, want the workload's 127", out.ExitCode)
	}
}

func TestAnMXCBackendFailureIsReportedAsALaunchFailureNotAWorkloadExit(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeBackendFails), "exits-zero.mjs")
	_, out, err := h.run(t)
	if !errors.Is(err, ErrLaunchFailed) {
		t.Fatalf("Wait error = %v, want ErrLaunchFailed", err)
	}
	if out.ExitCode != 127 {
		t.Errorf("exit code = %d, want the backend's 127 carried alongside the error", out.ExitCode)
	}
}

func TestBackendFailedRecognisesOnlyTheMXCReportAtExitCode127(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  sandbox.Outcome
		want bool
	}{
		{"the mxc report at 127", sandbox.Outcome{ExitCode: 127, Output: fakeBackendReport}, true},
		{"the mxc report at 126", sandbox.Outcome{ExitCode: 126, Output: fakeBackendReport}, false},
		{"the mxc report at 128", sandbox.Outcome{ExitCode: 128, Output: fakeBackendReport}, false},
		{"another error code at 127", sandbox.Outcome{ExitCode: 127, Output: `{"error":{"code":"policy_error"}}`}, false},
		{"plain text at 127", sandbox.Outcome{ExitCode: 127, Output: "node: command not found"}, false},
		{"no output at 127", sandbox.Outcome{ExitCode: 127}, false},
		{"workload text before the report at 127", sandbox.Outcome{ExitCode: 127, Output: "starting\n" + fakeBackendReport}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := backendFailed(tc.out); got != tc.want {
				t.Errorf("backendFailed(%+v) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestHealthyIsFalseWhenTheRuntimeCannotStartInsideMXC(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeRuntimeFails), "exits-zero.mjs")
	if h.driver.Healthy(context.Background()) {
		t.Error("Healthy() = true on a node whose mxc starts but whose runtime exits non-zero inside it")
	}
}

func TestHealthyIsTrueWhenTheRuntimeStartsInsideMXC(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	if !h.driver.Healthy(context.Background()) {
		t.Error("Healthy() = false although node --version exits 0 inside mxc")
	}
}

func TestHealthyReusesItsVerdictWithinTheCacheWindow(t *testing.T) {
	fake := fakeMXC(t, fakeWorks)
	h := newHarness(t, fake, "exits-zero.mjs")
	first := h.driver.Healthy(context.Background())
	second := h.driver.Healthy(context.Background())
	if !first || !second {
		t.Fatalf("Healthy() = %v then %v, want true twice", first, second)
	}
	if got := fakeLaunches(t, fake); got != 1 {
		t.Errorf("mxc was launched %d times for two health checks inside the cache window, want 1", got)
	}
}

func TestIsolationIsDeclaredWeak(t *testing.T) {
	h := newHarness(t, fakeMXC(t, fakeWorks), "exits-zero.mjs")
	if got := h.driver.Isolation(); got != sandbox.IsolationWeak {
		t.Errorf("Isolation() = %q, want %q", got, sandbox.IsolationWeak)
	}
}

func TestNewRefusesAnMXCExecutableItCannotRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mxcBin string
	}{
		{"no path", ""},
		{"a path that does not exist", filepath.Join(t.TempDir(), "missing-mxc")},
		{"a directory", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Config{MXCBin: tc.mxcBin, RunnerScript: testdataScript(t, "exits-zero.mjs"), BaseDir: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), "mxc executable") {
				t.Fatalf("New(MXCBin=%q) error = %v, want a refusal naming the mxc executable", tc.mxcBin, err)
			}
		})
	}
}

func TestJoinCommandLineQuotesOnlyArgumentsThatNeedIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"plain paths", []string{"/usr/bin/node", "/srv/run.mjs"}, "/usr/bin/node /srv/run.mjs"},
		{"a path with a space", []string{`C:\Program Files\nodejs\node.exe`, `C:\srv\run.mjs`}, `"C:\Program Files\nodejs\node.exe" C:\srv\run.mjs`},
		{"a path with a tab", []string{"/a\tb/node"}, "\"/a\tb/node\""},
		{"an empty argument", []string{"node", ""}, `node ""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := joinCommandLine(tc.args)
			if err != nil {
				t.Fatalf("joinCommandLine(%q): %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("joinCommandLine(%q) = %s, want %s", tc.args, got, tc.want)
			}
		})
	}
}

func TestJoinCommandLineRefusesAnArgumentWithADoubleQuote(t *testing.T) {
	if got, err := joinCommandLine([]string{"node", `run".mjs`}); err == nil {
		t.Fatalf("joinCommandLine accepted a double quote and produced %s", got)
	}
}
