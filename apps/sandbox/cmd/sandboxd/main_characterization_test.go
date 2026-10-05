package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/localdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const runMainEnv = "SKILLHUB_SANDBOXD_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func sandboxdCommand(t *testing.T, ctx context.Context, settings ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()
	env := []string{runMainEnv + "=1", "TEMP=" + tmp, "TMP=" + tmp, "TMPDIR=" + tmp, "SKILLHUB_SANDBOX_ADDR=127.0.0.1:0"}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "SKILLHUB_") || upper == "TEMP" || upper == "TMP" || upper == "TMPDIR" {
			continue
		}
		env = append(env, kv)
	}
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	env = append(env, settings...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	return cmd, &out
}

func TestSandboxdRefusesToStartOnAnInvalidNode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings []string
		want     string
	}{
		{
			name:     "no token, even with an unknown driver",
			settings: []string{"SKILLHUB_SANDBOX_DRIVER=podman"},
			want:     "SKILLHUB_SANDBOX_TOKEN is required",
		},
		{
			name:     "clean mode on runsc is refused before the driver is chosen",
			settings: []string{"SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_SANDBOX_RUNTIME=runsc", "SKILLHUB_CLEAN_MODE=1", "SKILLHUB_SANDBOX_DRIVER=podman"},
			want:     "SKILLHUB_CLEAN_MODE must not be set with runsc",
		},
		{
			name:     "an unknown driver",
			settings: []string{"SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_SANDBOX_DRIVER=podman"},
			want:     "is not a driver this node has",
		},
		{
			name:     "an egress network with no rendered allow list",
			settings: []string{"SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_CLEAN_MODE=1", "SKILLHUB_SANDBOX_NETWORK=skillhub_egress"},
			want:     "SKILLHUB_SANDBOX_EGRESS_ALLOW is required when SKILLHUB_SANDBOX_NETWORK is set",
		},
		{
			name: "an egress allow list that cannot be read",
			settings: []string{"SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_CLEAN_MODE=1", "SKILLHUB_SANDBOX_NETWORK=skillhub_egress",
				"SKILLHUB_SANDBOX_EGRESS_ALLOW=does-not-exist.json"},
			want: "could not load the rendered egress allow list",
		},
		{
			name: "a P-02 target the probe cannot dial",
			settings: []string{"SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_CLEAN_MODE=1",
				"SKILLHUB_SANDBOX_P02_TARGETS=db.internal:5432,cache.internal"},
			want: "SKILLHUB_SANDBOX_P02_TARGETS has entries that are not host:port (cache.internal)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd, out := sandboxdCommand(t, ctx, tc.settings...)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("sandboxd ended with %v, want exit status 1; output:\n%s", err, out)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("sandboxd output does not say %q:\n%s", tc.want, out)
			}
		})
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func capabilityOf(ctx context.Context, addr, token string) (sandbox.ProviderCapability, error) {
	var capability sandbox.ProviderCapability
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/capability", nil)
	if err != nil {
		return capability, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return capability, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return capability, errors.New(resp.Status)
	}
	return capability, json.NewDecoder(resp.Body).Decode(&capability)
}

func TestACleanNodeDeclaresWhatItCannotEnforce(t *testing.T) {
	addr := freeLoopbackAddr(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd, out := sandboxdCommand(t, ctx, "SKILLHUB_SANDBOX_TOKEN=t", "SKILLHUB_CLEAN_MODE=1", "SKILLHUB_SANDBOX_ADDR="+addr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var stopped sync.Once
	stop := func() {
		stopped.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}
	t.Cleanup(stop)

	var capability sandbox.ProviderCapability
	var err error
	for {
		if capability, err = capabilityOf(ctx, addr, "t"); err == nil {
			break
		}
		if ctx.Err() != nil {
			stop()
			t.Fatalf("sandboxd never answered /capability: %v; output:\n%s", err, out)
		}
		time.Sleep(100 * time.Millisecond)
	}

	local := &localdrv.Driver{}
	if capability.Isolation.Strength != sandbox.IsolationNone {
		t.Errorf("isolation = %q, want none", capability.Isolation.Strength)
	}
	if capability.Isolation.ReapsDetachedDescendants != local.Reaping().Detached {
		t.Errorf("reaps_detached_descendants = %v, want the local driver's %v",
			capability.Isolation.ReapsDetachedDescendants, local.Reaping().Detached)
	}
	if !reflect.DeepEqual(capability.MaxResources, sandbox.DefaultLimits) {
		t.Errorf("max_resources = %+v, want the defaults %+v", capability.MaxResources, sandbox.DefaultLimits)
	}
	if want := unenforcedCeilings(local.ResourceEnforcement()); !slices.Equal(capability.MaxResourcesUnenforced, want) {
		t.Errorf("max_resources_unenforced = %v, want %v", capability.MaxResourcesUnenforced, want)
	}
	if capability.Network == nil {
		t.Fatal("a clean node declared no network capability")
	}
	if !slices.Equal(capability.Network.EgressModes, []string{"default_deny", "none"}) || !capability.Network.EgressUnenforced {
		t.Errorf("network = %+v, want default_deny and none declared as unenforced", *capability.Network)
	}
	stop()
	if !strings.Contains(out.String(), "sandbox provider listening") {
		t.Errorf("sandboxd did not log that it is listening:\n%s", out)
	}
}
