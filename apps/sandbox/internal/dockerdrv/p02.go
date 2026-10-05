package dockerdrv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	networktypes "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func (d *Driver) ProbeEgress(ctx context.Context, targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	script, err := probeScript(targets)
	if err != nil {
		return nil, err
	}
	network := d.cfg.Network
	if network == "" {
		network = networktypes.NetworkNone
	}
	if network == networktypes.NetworkNone {

		return nil, nil
	}

	cfg := &container.Config{
		Image: d.cfg.Image,

		Entrypoint: []string{"node", "-e"},
		Cmd:        []string{script},
		User:       fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),

		Labels:          map[string]string{labelProbe: "p02"},
		Tty:             false,
		OpenStdin:       false,
		NetworkDisabled: false,
	}
	hc := &container.HostConfig{

		ReadonlyRootfs: true,
		Tmpfs:          map[string]string{"/tmp": "rw,nosuid,nodev,size=1m,noexec"},
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges:true"},
		Privileged:     false,
		NetworkMode:    container.NetworkMode(network),
		Runtime:        d.cfg.Runtime,
		AutoRemove:     false,
		Resources: container.Resources{
			Memory:    probeMemoryBytes,
			PidsLimit: ptr(hostPidsLimit(d.cfg.Runtime, sandbox.DefaultLimits.MaxPIDs)),
		},
		LogConfig: container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "1m", "max-file": "1"}},
	}

	const probeName = "skillhub-p02-probe"

	_, _ = d.cli.ContainerRemove(ctx, probeName, client.ContainerRemoveOptions{Force: true})
	created, err := d.cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: cfg, HostConfig: hc, Name: probeName})
	if err != nil {
		return nil, fmt.Errorf("create p02 probe: %w", err)
	}
	defer func() {

		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = d.cli.ContainerRemove(cleanup, created.ID, client.ContainerRemoveOptions{Force: true})
	}()
	if _, err := d.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start p02 probe: %w", err)
	}

	var exitCode int64
	wait := d.cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-wait.Error:
		return nil, fmt.Errorf("p02 probe did not finish: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-wait.Result:
		exitCode = result.StatusCode
	}

	out, err := d.probeLogs(ctx, created.ID)
	if err != nil {

		return nil, fmt.Errorf("read p02 probe output: %w", err)
	}
	return probeVerdict(exitCode, out, targets)
}

const probeDoneMarker = "P02-DONE "

func probeVerdict(exitCode int64, out string, targets []string) ([]string, error) {
	if exitCode != 0 {
		return nil, fmt.Errorf("p02 probe did not finish: container exited with code %d", exitCode)
	}
	done := -1
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), probeDoneMarker); ok {
			if n, err := strconv.Atoi(rest); err == nil {
				done = n
			}
		}
	}
	if done != len(targets) {
		return nil, fmt.Errorf("p02 probe did not finish: completion marker reports %d of %d targets", done, len(targets))
	}
	return parseProbeOutput(out, targets), nil
}

func (d *Driver) probeLogs(ctx context.Context, id string) (string, error) {
	rc, err := d.cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, probeLogLimit)); err != nil && buf.Len() == 0 {
		return "", err
	}
	return buf.String(), nil
}

func probeScript(targets []string) (string, error) {
	type target struct {
		Label string `json:"t"`
		Host  string `json:"h"`
		Port  int    `json:"p"`
	}
	var list []target
	for _, t := range targets {
		host, port, ok := sandbox.SplitP02Target(t)
		if !ok {
			return "", fmt.Errorf("p02 target %q is not host:port", t)
		}
		list = append(list, target{Label: t, Host: host, Port: port})
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("encode p02 targets: %w", err)
	}

	return fmt.Sprintf(probeSource, encoded, probeDialTimeoutMS), nil
}

const probeDialTimeoutMS = 2000

const (
	probeMemoryBytes = 64 << 20
	probeLogLimit    = 64 << 10
)

const probeSource = `
const net = require("node:net");
const targets = %s;
let pending = targets.length;
const complete = () => { process.stdout.write("P02-DONE " + targets.length + "\n"); process.exit(0); };
const finish = () => { if (--pending <= 0) complete(); };
if (pending === 0) complete();
for (const t of targets) {
  let settled = false;
  const done = () => { if (!settled) { settled = true; finish(); } };
  const socket = net.connect({ host: t.h, port: t.p });
  const timer = setTimeout(() => { socket.destroy(); done(); }, %d);
  socket.on("connect", () => {
    process.stdout.write("REACHED " + t.t + "\n");
    clearTimeout(timer);
    socket.destroy();
    done();
  });
  socket.on("error", () => { clearTimeout(timer); done(); });
}
`

func parseProbeOutput(out string, targets []string) []string {
	want := map[string]bool{}
	for _, t := range targets {
		want[t] = true
	}
	var reached []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "REACHED ")
		if !ok {
			continue
		}
		if want[rest] {
			want[rest] = false
			reached = append(reached, rest)
		}
	}
	return reached
}

func ptr[T any](v T) *T { return &v }

var _ sandbox.EgressProber = (*Driver)(nil)
