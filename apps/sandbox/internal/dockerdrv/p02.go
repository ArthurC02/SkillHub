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

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func (d *Driver) ProbeEgress(ctx context.Context, targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	network := d.cfg.Network
	if network == "" {
		network = "none"
	}
	if network == "none" {

		return nil, nil
	}

	script, err := probeScript(targets)
	if err != nil {
		return nil, err
	}
	cfg := &container.Config{
		Image: d.cfg.Image,

		Entrypoint: strslice.StrSlice{"node", "-e"},
		Cmd:        strslice.StrSlice{script},
		User:       fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),

		Labels:          map[string]string{labelProbe: "p02"},
		Tty:             false,
		OpenStdin:       false,
		NetworkDisabled: false,
	}
	hc := &container.HostConfig{

		ReadonlyRootfs: true,
		Tmpfs:          map[string]string{"/tmp": "rw,nosuid,nodev,size=1m,noexec"},
		CapDrop:        strslice.StrSlice{"ALL"},
		SecurityOpt:    []string{"no-new-privileges:true"},
		Privileged:     false,
		NetworkMode:    container.NetworkMode(network),
		Runtime:        d.cfg.Runtime,
		AutoRemove:     false,
		Resources: container.Resources{
			Memory:    64 << 20,
			PidsLimit: ptr(int64(32)),
		},
		LogConfig: container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "1m", "max-file": "1"}},
	}

	const probeName = "skillhub-p02-probe"

	_ = d.cli.ContainerRemove(ctx, probeName, container.RemoveOptions{Force: true})
	created, err := d.cli.ContainerCreate(ctx, cfg, hc, nil, nil, probeName)
	if err != nil {
		return nil, fmt.Errorf("create p02 probe: %w", err)
	}
	defer func() {

		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.cli.ContainerRemove(cleanup, created.ID, container.RemoveOptions{Force: true})
	}()
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start p02 probe: %w", err)
	}

	statusCh, errCh := d.cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("p02 probe did not finish: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-statusCh:
	}

	out, err := d.probeLogs(ctx, created.ID)
	if err != nil {

		return nil, fmt.Errorf("read p02 probe output: %w", err)
	}
	return parseProbeOutput(out, targets), nil
}

func (d *Driver) probeLogs(ctx context.Context, id string) (string, error) {
	rc, err := d.cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, 64<<10)); err != nil && buf.Len() == 0 {
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
		// LastIndex, so an IPv6 host keeps its own colons and only the port splits off.
		i := strings.LastIndex(t, ":")
		if i <= 0 {
			continue
		}
		port, err := strconv.Atoi(t[i+1:])
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		list = append(list, target{Label: t, Host: t[:i], Port: port})
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("encode p02 targets: %w", err)
	}

	return fmt.Sprintf(probeSource, encoded, probeDialTimeoutMS), nil
}

const probeDialTimeoutMS = 2000

const probeSource = `
const net = require("node:net");
const targets = %s;
let pending = targets.length;
const finish = () => { if (--pending <= 0) process.exit(0); };
if (pending === 0) process.exit(0);
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
