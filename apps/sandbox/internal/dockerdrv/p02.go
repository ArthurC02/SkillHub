package dockerdrv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// ProbeEgress is the driver's half of ADR-022 T10: try to open a TCP connection
// to each forbidden address FROM A SANDBOX'S OWN NETWORK POSITION, and report
// which ones answered.
//
// The network position is the entire measurement. sandboxd runs on the node
// with the node's own access and can reach the database; dialling from this
// process would answer a question nobody asked, and would answer it the wrong
// way round forever. So the attempt is made from a throwaway container built
// the way a run's sandbox is built - same runtime, same network, same
// capability set - and what it can reach is what a run can reach.
//
// It deliberately does NOT go through Start/Wait. Those create a ProviderRun:
// an entry in the manager, a slot consumed, a handle the platform can list and
// would then have to explain. This is a probe, not a run, and it is invisible to
// GET /runs by construction rather than by filtering.
func (d *Driver) ProbeEgress(ctx context.Context, targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	network := d.cfg.Network
	if network == "" {
		network = "none"
	}
	if network == "none" {
		// A node with no egress network gives its sandboxes no interface at all.
		// There is nothing to dial from and nothing that could be reached, so
		// the answer is "nothing was reachable" rather than an error: reporting
		// `unknown` here would take a correctly-isolated node out of rotation.
		return nil, nil
	}

	script := probeScript(targets)
	cfg := &container.Config{
		Image: d.cfg.Image,
		// The image's own entrypoint is the agent harness. This is not a run.
		Entrypoint: strslice.StrSlice{"/bin/sh", "-c"},
		Cmd:        strslice.StrSlice{script},
		User:       fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
		// Deliberately NOT labelManaged. Adopt() rebuilds runs from that label
		// after a restart, so a probe container carrying it would come back as
		// a phantom ProviderRun - listed by GET /runs, counted against a slot,
		// and reported to the platform's orphan scan as a leak. The probe is
		// removed by name at the start of the next round, which is what handles
		// one left behind by a killed sandboxd.
		Labels:          map[string]string{labelProbe: "p02"},
		Tty:             false,
		OpenStdin:       false,
		NetworkDisabled: false,
	}
	hc := &container.HostConfig{
		// The same isolation a run gets. A probe running with more than a run
		// would measure a boundary nothing else is behind; with less, it would
		// report a block that is only this container's.
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
	// A probe container left behind by a killed sandboxd would block the next
	// one by name. Removing first is idempotent and cheaper than generating a
	// handle nobody will ever look up.
	_ = d.cli.ContainerRemove(ctx, probeName, container.RemoveOptions{Force: true})
	created, err := d.cli.ContainerCreate(ctx, cfg, hc, nil, nil, probeName)
	if err != nil {
		return nil, fmt.Errorf("create p02 probe: %w", err)
	}
	defer func() {
		// Its own context: the caller's is already cancelled on the timeout path,
		// and a probe that leaves a container behind on every timeout is a leak
		// the reconciler would then report as an orphan.
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
		// Not a pass. parseProbeOutput("") is "nothing was reachable", so a
		// transcript this code cannot read has to become an error here or the
		// probe reports a clean result for a reading it never saw.
		//
		// The first version of this line called d.tail, which prefixes the
		// handle with "skillhub-run-" on its way to ContainerLogs. It would have
		// queried a container that does not exist, got an error, returned "",
		// and reported PASS on every round forever.
		return nil, fmt.Errorf("read p02 probe output: %w", err)
	}
	return parseProbeOutput(out, targets), nil
}

// probeLogs reads the probe container's transcript by ID.
//
// Not d.tail: that one takes a provider_run_id and prefixes it, because
// everything else it reads is a run. The probe is not a run and does not have
// one of those handles.
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

// probeScript writes one line per target that ANSWERED. Silence is the pass.
//
// A script that reported both outcomes would make an empty transcript - the
// shape a container that failed to start produces - indistinguishable from a
// clean pass, and the clean pass is the common case. So the caller can tell the
// difference: no REACHED lines and a completed container is a pass, and a
// container that did not complete is an error the probe reports as `unknown`.
func probeScript(targets []string) string {
	var b strings.Builder
	// -w 2: a target that is filtered rather than refused hangs, and the probe
	// has a whole-round timeout it must not spend on one address.
	for _, t := range targets {
		host, port, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "nc -z -w 2 %s %s 2>/dev/null && echo REACHED %s:%s\n",
			shellQuote(host), shellQuote(port), host, port)
	}
	b.WriteString("exit 0\n")
	return b.String()
}

// shellQuote is single-quote wrapping, and the targets come from node
// configuration rather than from a RunRequest - but this string is assembled
// into a shell command, so it is quoted at the boundary regardless of who wrote
// the value.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseProbeOutput keeps only the targets the probe actually named, so a
// workload-shaped surprise in the transcript cannot invent a breach.
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
			want[rest] = false // each target counts once
			reached = append(reached, rest)
		}
	}
	return reached
}

func ptr[T any](v T) *T { return &v }

var _ sandbox.EgressProber = (*Driver)(nil)
