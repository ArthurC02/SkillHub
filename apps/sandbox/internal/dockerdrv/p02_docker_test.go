package dockerdrv_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
)

func TestProbeEgressAgainstARealListener(t *testing.T) {
	cli := dockerClient(t)
	img := probeImage(t, cli)

	ip, port := startListener(t, cli, img)
	open := ip + ":" + port
	closed := ip + ":1"

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

func probeImage(t *testing.T, cli *client.Client) string {
	t.Helper()
	const runtimeImage = "skillhub/runtime-agent-sdk:2026.08-10"
	img := runtimeImage
	if v := os.Getenv("SKILLHUB_SANDBOX_TEST_IMAGE"); v != "" {
		img = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ImageInspect(ctx, img); err != nil {

		skipOrFail(t, "the P-02 probe test needs the Runtime Image locally (%s, or set "+
			"SKILLHUB_SANDBOX_TEST_IMAGE); this assertion is about what that image can dial, "+
			"and the default busybox test image would pass it for the wrong reason: %v", img, err)
	}
	return img
}

func startListener(t *testing.T, cli *client.Client, img string) (ip, port string) {
	t.Helper()
	const listenPort = "17777"
	const src = `const net=require("node:net");` +
		`net.createServer(s=>s.end()).listen(` + listenPort + `,"0.0.0.0",()=>console.log("LISTENING"));`

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      img,
			Entrypoint: []string{"node", "-e"},
			Cmd:        []string{src},
			Labels:     map[string]string{testLabel: "1"},
		},
		HostConfig: &container.HostConfig{NetworkMode: "bridge", Runtime: testRuntime()},
	})
	if err != nil {
		t.Fatalf("create listener: %v", err)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_, _ = cli.ContainerRemove(rmCtx, created.ID, client.ContainerRemoveOptions{Force: true})
	})
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start listener: %v", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		inspected, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
		if err != nil {
			t.Fatalf("inspect listener: %v", err)
		}
		insp := inspected.Container
		logs := containerLogs(t, cli, created.ID)
		if strings.Contains(logs, "LISTENING") {

			addr := ""
			for _, n := range insp.NetworkSettings.Networks {
				if n.IPAddress.IsValid() {
					addr = n.IPAddress.String()
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

func containerLogs(t *testing.T, cli *client.Client, id string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rc, err := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return ""
	}
	defer rc.Close()
	var buf bytes.Buffer
	_, _ = stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, 32<<10))
	return buf.String()
}
