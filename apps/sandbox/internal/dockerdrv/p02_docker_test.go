package dockerdrv_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
)

// TestProbeEgressAgainstARealListener is the half of ADR-022 T10 that had no
// test at all: every case in sandbox/p02_test.go drives a fakeProber, so the
// state machine was measured and the probe itself never was.
//
// That gap is not theoretical. The probe used to shell out to `nc -z`, and the
// Runtime Image has no `nc` — the `&&` short-circuited, nothing was printed,
// and "no REACHED lines" is exactly what a clean pass looks like. A probe that
// cannot fail is not a probe, and the default test image (busybox) would have
// hidden it, because busybox builds nc in. So this test insists on an image
// that is actually the Runtime Image, opens a listener the probe must find,
// and asks for a closed port in the same round that the probe must not claim.
func TestProbeEgressAgainstARealListener(t *testing.T) {
	cli := dockerClient(t)
	img := probeImage(t, cli)

	// The listener lives in its own container on the default bridge, not on
	// the test process's host port: the whole measurement is "what can a
	// sandbox reach from a sandbox's network position", and a host port is
	// reachable by a different route on every platform this suite runs on.
	ip, port := startListener(t, cli, img)
	open := ip + ":" + port
	closed := ip + ":1" // nothing binds port 1 in that container

	d, err := dockerdrv.New(dockerdrv.Config{
		Image:       img,
		Network:     "bridge",
		UID:         65532,
		GID:         65532,
		Runtime:     testRuntime(),
		ExtraLabels: map[string]string{testLabel: "1"},
	})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reached, err := d.ProbeEgress(ctx, []string{open, closed})
	if err != nil {
		t.Fatalf("ProbeEgress: %v", err)
	}

	if len(reached) != 1 || reached[0] != open {
		t.Fatalf("ProbeEgress(%v) = %v, want exactly [%s]: the open listener must be reported, "+
			"and a probe that reports nothing is indistinguishable from a pass", []string{open, closed}, reached, open)
	}
}

// probeImage insists on an image that carries the workload runtime. Skipping
// loudly is the point: silently falling back to the default test image is how
// the `nc` bug would have stayed green, since busybox has the tool the real
// image does not.
func probeImage(t *testing.T, cli *client.Client) string {
	t.Helper()
	const runtimeImage = "skillhub/runtime-agent-sdk:2026.08-3"
	img := runtimeImage
	if v := os.Getenv("SKILLHUB_SANDBOX_TEST_IMAGE"); v != "" {
		img = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ImageInspect(ctx, img); err != nil {
		// Not pulled: the Runtime Image is built by its own pipeline and is
		// hundreds of megabytes, so this test uses what is already on the
		// machine rather than fetching it.
		skipOrFail(t, "the P-02 probe test needs the Runtime Image locally (%s, or set "+
			"SKILLHUB_SANDBOX_TEST_IMAGE); this assertion is about what that image can dial, "+
			"and the default busybox test image would pass it for the wrong reason: %v", img, err)
	}
	return img
}

// startListener runs one container that binds a port and announces it, and
// returns the address a peer on the same network can dial.
func startListener(t *testing.T, cli *client.Client, img string) (ip, port string) {
	t.Helper()
	const listenPort = "17777"
	const src = `const net=require("node:net");` +
		`net.createServer(s=>s.end()).listen(` + listenPort + `,"0.0.0.0",()=>console.log("LISTENING"));`

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:      img,
			Entrypoint: strslice.StrSlice{"node", "-e"},
			Cmd:        strslice.StrSlice{src},
			Labels:     map[string]string{testLabel: "1"},
		},
		&container.HostConfig{NetworkMode: "bridge", Runtime: testRuntime()},
		nil, nil, "")
	if err != nil {
		t.Fatalf("create listener: %v", err)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = cli.ContainerRemove(rmCtx, created.ID, container.RemoveOptions{Force: true})
	})
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start listener: %v", err)
	}

	// Wait for the socket to be bound rather than for the container to be
	// running: a probe that arrives first would report a clean pass for a
	// listener that simply was not up yet.
	deadline := time.Now().Add(60 * time.Second)
	for {
		insp, err := cli.ContainerInspect(ctx, created.ID)
		if err != nil {
			t.Fatalf("inspect listener: %v", err)
		}
		logs := containerLogs(t, cli, created.ID)
		if strings.Contains(logs, "LISTENING") {
			// NetworkSettings.IPAddress is deprecated (gone in docker v29); the
			// address lives on the default network entry.
			addr := ""
			for _, n := range insp.NetworkSettings.Networks {
				if n.IPAddress != "" {
					addr = n.IPAddress
					break
				}
			}
			if addr == "" {
				t.Fatalf("listener container has no IP address on the bridge network")
			}
			return addr, listenPort
		}
		if !insp.State.Running || time.Now().After(deadline) {
			t.Fatalf("listener never announced itself (running=%v): %s", insp.State.Running, logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// containerLogs reads a container's transcript, demultiplexing the daemon's
// stdout/stderr framing. Best effort: an unreadable log means "not yet", which
// the caller's deadline turns into a failure rather than a pass.
func containerLogs(t *testing.T, cli *client.Client, id string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rc, err := cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return ""
	}
	defer rc.Close()
	var buf bytes.Buffer
	_, _ = stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, 32<<10))
	return buf.String()
}
