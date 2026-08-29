// Package dockerdrv is the development SandboxProvider driver: one Docker
// container per attempt, with the ADR-005 baseline applied to every one of
// them.
//
// It is the dev implementation of what production runs as gVisor. gVisor needs
// Linux and the development machine is Windows (docs/plans/mvp/m2/README.md), so the
// same code path sets HostConfig.Runtime to "runsc" from configuration and the
// isolation level this provider declares follows that setting. Everything else
// — non-root, read-only rootfs, dropped capabilities, no management socket, no
// host mounts, resource ceilings, no network — is identical in both, because
// ADR-015 makes gVisor an extra layer rather than a replacement for any of it.
package dockerdrv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// Paths inside the sandbox. /work is the agent's working directory, and the
// skill unpacks to <workdir>/.claude/skills/<name>/ — the layout the PDM-003
// spike measured, not the `skills/` one the proposal had assumed.
const (
	WorkDir = "/work"
	OutDir  = "/out"

	labelManaged   = "skillhub.sandbox.managed"
	labelRunID     = "skillhub.sandbox.run_id"
	labelAttemptID = "skillhub.sandbox.run_attempt_id"
	labelAttempt   = "skillhub.sandbox.attempt"
	labelWorkspace = "skillhub.sandbox.workspace_id"
	labelHandle    = "skillhub.sandbox.provider_run_id"
	// labelProbe marks the P-02 probe container (ADR-022 T10). It is a
	// different label from labelManaged on purpose: the probe is not a run, and
	// Adopt() must not rebuild it as one.
	labelProbe    = "skillhub.sandbox.probe"
	labelHash     = "skillhub.sandbox.request_hash"
	labelDeadline = "skillhub.sandbox.hard_deadline"

	// logTailBytes bounds what a workload can push into the result. The full
	// output belongs in Trace (TRACE-004), not in a provider answer.
	logTailBytes = 32 << 10

	// TracePath is where the runtime image's harness appends its trace events,
	// one JSON object per line (TRACE-002). It is under /out because that tmpfs
	// is the one place the workload is allowed to write that the driver can
	// still read after the process has exited.
	TracePath = OutDir + "/trace/events.jsonl"
	// traceReadLimit bounds one read out of a sandbox. The tmpfs is already
	// sized from the run's disk ceiling; this is the second bound, applied to
	// bytes that are about to enter this process (iron rule 1).
	traceReadLimit = 8 << 20
)

// Config is deployment policy for the driver.
type Config struct {
	// Image is the pre-built, versioned Runtime Image (I-01). Pin it by digest
	// in production (I-02); a tag is a moving target and the run record would
	// no longer say what actually ran.
	Image string
	// Runtime is the container runtime name. Empty means the host default,
	// which is what a developer machine has; production sets "runsc" (ADR-015).
	Runtime string
	// Network is the egress network a sandbox joins *when its RunRequest allows
	// a destination* (SBX-007). Empty or "none" means this node has no egress
	// route at all and every sandbox runs on `--network none`, which is strictly
	// stronger than default-deny with an empty allow list.
	//
	// In development the network is a Docker network with `internal: true` on
	// which the LiteLLM gateway is the only reachable address - an allow list of
	// one, enforced by what is on the wire rather than by a proxy. Production
	// keeps that shape and changes only the enforcement: ADR-022 Q3 decided
	// nftables default-deny plus a node-pinned DNS resolver and *no* proxy, so
	// there is no proxy network to name here. What this names in production is
	// the run's egress network; the destination list, the pinned IP and port,
	// and the logging of refused attempts all live in the node's ruleset,
	// rendered from infra/egress/allowlist.yaml at node build time and never
	// edited on the node.
	//
	// Either way a run whose egress allows nothing gets no network, and object
	// storage is never on this wire: the node moves those bytes itself.
	Network string
	// UID/GID the workload runs as. Never 0 (C-02).
	UID, GID int
	// StorageQuota turns on the storage driver's per-container size option. It
	// needs xfs with pquota or btrfs, so it is off by default and the writable
	// space is bounded by the tmpfs sizes instead.
	StorageQuota bool
	// AllowDevCmd lets a RunRequest override the image's command through
	// provider_extensions.dev_cmd. Development and the isolation tests only:
	// it grants nothing the untrusted workload does not already have, but a
	// production provider should not accept caller-chosen entrypoints.
	AllowDevCmd bool
	// ExtraLabels are stamped onto every container, so a test run can find and
	// remove exactly its own.
	ExtraLabels map[string]string
}

type Driver struct {
	cli *client.Client
	cfg Config
}

func New(cfg Config) (*Driver, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	if cfg.UID == 0 || cfg.GID == 0 {
		return nil, errors.New("sandbox uid/gid must not be root (baseline C-02)")
	}
	if cfg.Image == "" {
		return nil, errors.New("a pinned runtime image is required (baseline I-01)")
	}
	if cfg.Network == "" {
		cfg.Network = "none"
	}
	return &Driver{cli: cli, cfg: cfg}, nil
}

func (d *Driver) Close() error { return d.cli.Close() }

func name(providerRunID string) string { return "skillhub-run-" + providerRunID }

// Start creates and starts one sandbox. Every isolation setting is applied
// here, in one reviewable block, because a baseline spread across helpers is a
// baseline nobody can check against the SEC-002 table.
func (d *Driver) Start(ctx context.Context, id string, req sandbox.RunRequest) error {
	lim := req.ResourceLimits
	// PDM-005 5.2 splits the disk ceiling /work 6 GiB + /out 2 GiB. tmpfs
	// enforces both, and since tmpfs pages count against the memory cgroup, a
	// workload trying to fill them hits the memory limit first — stricter than
	// the disk ceiling, never looser.
	workBytes := lim.DiskBytes * 3 / 4
	outBytes := lim.DiskBytes - workBytes
	mount := func(size int64, extra string) string {
		return fmt.Sprintf("rw,nosuid,nodev,size=%d,uid=%d,gid=%d,mode=0700%s", size, d.cfg.UID, d.cfg.GID, extra)
	}

	// SBX-007. A sandbox joins the egress network only when its own policy names
	// a destination; anything else runs with no network at all.
	network := d.networkFor(req)

	cfg := &container.Config{
		Image:      d.cfg.Image,
		User:       fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID), // C-02: never root
		Env:        env(req),
		WorkingDir: WorkDir,
		Labels:     d.labels(req, id),
		// The workload gets no stdin and no tty: it is a batch job, and a tty
		// would merge its streams and give it terminal control sequences to
		// play with on the way back out (ADR-009).
		Tty:             false,
		OpenStdin:       false,
		NetworkDisabled: network == "none",
	}
	if cmd, ok := devCmd(req); ok && d.cfg.AllowDevCmd {
		cfg.Cmd = cmd
	}

	pids := lim.MaxPIDs
	hc := &container.HostConfig{
		// C-06: nothing but the declared scratch paths is writable.
		ReadonlyRootfs: true,
		Tmpfs: map[string]string{
			WorkDir: mount(workBytes, ""),
			OutDir:  mount(outBytes, ""),
			// /tmp holds no skill code, so it can refuse execution outright.
			"/tmp": mount(64<<20, ",noexec"),
		},
		// C-08 / C-02: no capabilities at all, and no way to gain any.
		CapDrop:     strslice.StrSlice{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Privileged:  false,
		// C-04: the sandbox shares no namespace with the host or with another
		// run. Left at the zero value deliberately — naming them is how they
		// get set, and there is nothing here to set them to.
		//
		// C-05 / C-07: Binds and Mounts stay empty. No docker socket, no host
		// path, no node credentials. The only bytes that reach a sandbox come
		// through the short-lived grants in the RunRequest (SBX-008).
		NetworkMode: container.NetworkMode(network),
		Runtime:     d.cfg.Runtime, // "runsc" in production (ADR-015)
		AutoRemove:  false,         // teardown is DELETE, and it must be explicit
		Resources: container.Resources{
			NanoCPUs:   int64(lim.VCPU * 1e9), // C-10
			Memory:     lim.MemoryBytes,       // C-11
			MemorySwap: lim.MemoryBytes,       // no swap: the ceiling is the ceiling
			PidsLimit:  &pids,                 // C-13, stops a fork bomb
			Ulimits: []*container.Ulimit{ // C-14
				{Name: "nofile", Soft: lim.MaxOpenFiles, Hard: lim.MaxOpenFiles},
				// C-16: no core dumps filling the scratch space on a crash.
				{Name: "core", Soft: 0, Hard: 0},
			},
		},
		// A workload that logs in a loop must not fill the node's disk.
		LogConfig: container.LogConfig{
			Type:   "json-file",
			Config: map[string]string{"max-size": "16m", "max-file": "1"},
		},
	}
	if d.cfg.StorageQuota {
		hc.StorageOpt = map[string]string{"size": strconv.FormatInt(lim.DiskBytes, 10)} // C-12
	}

	created, err := d.cli.ContainerCreate(ctx, cfg, hc, nil, nil, name(id))
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		// Leave the container in place: DELETE is what releases it, and a
		// half-created sandbox that vanishes on its own is one the platform
		// cannot reconcile.
		return fmt.Errorf("start sandbox: %w", err)
	}
	// SBX-008: the inputs go in after the start, and the workload waits for the
	// ready marker before it touches any of them. The same failure leaves the
	// container in place for the same reason.
	return d.pushInputs(ctx, id, req)
}

// networkFor decides what one sandbox is connected to (SBX-007).
//
// The read is one way only, and it is the same ordering the contract states for
// `none` versus `default_deny`: a run that is allowed to reach nothing gets no
// network, whatever this node could have offered, and a run that names a
// destination gets the egress network *only if this node has one*. A node
// without an egress network never substitutes a weaker isolation for a stronger
// one - it declares only `none` in its capability and the scheduler does not
// send it work that needs a route (RUN-005).
func (d *Driver) networkFor(req sandbox.RunRequest) string {
	if d.cfg.Network == "" || d.cfg.Network == "none" {
		return "none"
	}
	// Any named destination attaches the network, and this used to test for the
	// literal "model_gateway" instead. That was the only place in the repo that
	// read `Egress.Allow` at all, and reading only the purpose meant a request
	// naming any host while calling itself model_gateway got the network, while
	// a legitimate second purpose silently got none.
	//
	// Neither is possible now: Config.accept refuses any destination this node
	// has no rendered rule for (ADR-022 A1-e) and it runs before Create reaches
	// Start, so by here every entry names a route this node actually has. The
	// question left is the one this function is for -- attach the network or
	// not -- and a run allowed to reach nothing still gets none.
	if len(req.Egress.Allow) > 0 {
		return d.cfg.Network
	}
	return "none"
}

func (d *Driver) Wait(ctx context.Context, id string) (sandbox.Outcome, error) {
	statusCh, errCh := d.cli.ContainerWait(ctx, name(id), container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return sandbox.Outcome{}, err
	case <-ctx.Done():
		return sandbox.Outcome{}, ctx.Err()
	case st := <-statusCh:
		out := sandbox.Outcome{ExitCode: int(st.StatusCode)}
		if st.Error != nil {
			return out, errors.New(st.Error.Message)
		}
		// Inspect after the fact: OOMKilled is how the memory ceiling reports
		// itself, and an exit code alone cannot tell it from an ordinary crash.
		if insp, err := d.cli.ContainerInspect(context.WithoutCancel(ctx), name(id)); err == nil && insp.State != nil {
			out.OOMKilled = insp.State.OOMKilled
		}
		out.Output = d.tail(context.WithoutCancel(ctx), id)
		return out, nil
	}
}

// Stop asks the workload to stop and lets the runtime kill it after grace. It
// is the same call for a cancel and for the wall clock; only the grace differs.
func (d *Driver) Stop(ctx context.Context, id string, grace time.Duration) error {
	secs := int(grace.Seconds())
	if secs < 0 {
		secs = 0
	}
	err := d.cli.ContainerStop(ctx, name(id), container.StopOptions{Timeout: &secs})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return err
	}
	return nil
}

// Remove releases the sandbox and its scratch space. Force covers a container
// still running, and a missing one is success: destroy has no 404 (SBX-009).
func (d *Driver) Remove(ctx context.Context, id string) error {
	err := d.cli.ContainerRemove(ctx, name(id), container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return err
	}
	return nil
}

// Adopt reads back what the labels recorded, so a restarted provider knows what
// it still holds.
//
// ponytail: container labels are the only durable record this driver keeps —
// enough to answer GET /runs, destroy the sandbox and recognise a re-sent
// dispatch by hash, but not enough to reconstruct the RunRequest, and
// deliberately so: the request carries secrets and labels are world-readable
// on the node (D-05). A production provider that needs more keeps a local
// store on the execution node, never a core database connection (iron rule 2).
func (d *Driver) Adopt(ctx context.Context) ([]sandbox.Adopted, error) {
	list, err := d.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", labelManaged+"=1")),
	})
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Adopted, 0, len(list))
	for _, c := range list {
		attempt, _ := strconv.Atoi(c.Labels[labelAttempt])
		deadline, _ := time.Parse(time.RFC3339, c.Labels[labelDeadline])
		out = append(out, sandbox.Adopted{
			ProviderRunID: c.Labels[labelHandle],
			RunID:         c.Labels[labelRunID],
			RunAttemptID:  c.Labels[labelAttemptID],
			Attempt:       attempt,
			RequestHash:   c.Labels[labelHash],
			CreatedAt:     time.Unix(c.Created, 0).UTC(),
			Running:       c.State == "running",
			HardDeadline:  deadline,
		})
	}
	return out, nil
}

// ReadTrace reads the workload's trace file out of the sandbox.
//
// It runs bounded `dd` inside the container rather than copying the path out, and that
// is not a preference: /out is a tmpfs, and the Docker copy API resolves paths
// against the container's rootfs layer, so it answers "no such file" for
// anything under a mount. Verified against the daemon, not assumed.
//
// The alternatives were worse. A bind mount would put a host path inside the
// sandbox, which baseline C-05 forbids outright. Routing events through stdout
// would merge them with the workload's own output and give up the separation
// between script_log and the rest of the trace.
//
// What this does cost is an exec into an untrusted container. It is bounded to
// what that buys an attacker: an absolute path to the image's own binary on a
// read-only rootfs (so the workload cannot substitute it), no shell, no
// only a Manager-owned decimal offset, the same unprivileged uid the workload already
// runs as, and a bounded read of the result. The workload can make `dd` print
// whatever it likes - which is exactly what it could already do, since the file
// is its own output and is untrusted input to the platform either way.
//
// A missing file, a stopped container and an unreadable stream are all "nothing
// collected yet" rather than errors: the collector polls from the moment the
// sandbox starts and most early passes legitimately find nothing.
func (d *Driver) ReadTrace(ctx context.Context, id string, offset int64) ([]byte, bool, error) {
	const blockSize = int64(1 << 20)
	base := offset / blockSize * blockSize
	prefix := int(offset - base)
	exec, err := d.cli.ContainerExecCreate(ctx, name(id), container.ExecOptions{
		Cmd: []string{
			"/bin/dd", "if=" + TracePath, "bs=" + strconv.FormatInt(blockSize, 10),
			"skip=" + strconv.FormatInt(base/blockSize, 10), "count=9",
		},
		AttachStdout: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			return nil, false, nil // gone, or not running yet
		}
		return nil, false, err
	}
	attached, err := d.cli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, false, err
	}
	defer attached.Close()

	// Demultiplexed: stderr (the "No such file" of an empty run) is discarded, so
	// a missing file reads as no events rather than as a corrupt line.
	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, io.Discard, attached.Reader); err != nil {
		return nil, false, err
	}
	if prefix >= out.Len() {
		return nil, false, nil
	}
	data := out.Bytes()[prefix:]
	more := len(data) > traceReadLimit
	if more {
		data = data[:traceReadLimit]
	}
	return data, more, nil
}

func (d *Driver) Healthy(ctx context.Context) bool {
	_, err := d.cli.Ping(ctx)
	return err == nil
}

func (d *Driver) labels(req sandbox.RunRequest, id string) map[string]string {
	l := map[string]string{
		labelManaged:   "1",
		labelHandle:    id,
		labelRunID:     req.RunID,
		labelAttemptID: req.RunAttemptID,
		labelAttempt:   strconv.Itoa(req.Attempt),
		// Tenancy label for routing and reconciliation only. It grants nothing:
		// scope was settled in the control plane before this request was built
		// (iron rule 3).
		labelWorkspace: req.WorkspaceID,
		labelHash:      sandbox.HashRequest(req),
		labelDeadline: time.Now().UTC().
			Add(time.Duration(req.ResourceLimits.WallClockHardSeconds) * time.Second).
			Format(time.RFC3339),
	}
	for k, v := range d.cfg.ExtraLabels {
		l[k] = v
	}
	return l
}

// env hands the sandbox what it needs: paths, and the one credential it is meant
// to use. Object grant URLs are deliberately *not* here any more — the node
// fetches those itself and writes the bytes in (see transfer.go), so the only
// secret inside a sandbox is the credential for the only destination it can
// reach.
//
// The Virtual Key travels as ANTHROPIC_AUTH_TOKEN because the Claude Agent SDK
// reads that natively (PDM-003). It is short-lived, scoped to this run and
// revoked at teardown; it is also visible to the workload, which is why the
// budget and TTL on it are the actual control (ADR-017).
func env(req sandbox.RunRequest) []string {
	e := []string{
		"SKILLHUB_RUN_ID=" + req.RunID,
		"SKILLHUB_RUN_ATTEMPT_ID=" + req.RunAttemptID,
		"SKILLHUB_ATTEMPT=" + strconv.Itoa(req.Attempt),
		"SKILLHUB_WORKSPACE_ID=" + req.WorkspaceID,
		"SKILLHUB_WORKDIR=" + WorkDir,
		"SKILLHUB_OUTDIR=" + OutDir,
		"SKILLHUB_SKILL_DIR=" + WorkDir + "/.claude/skills",
		"SKILLHUB_INPUT_DIR=" + InputDir,
		"SKILLHUB_DATASET_DIR=" + DatasetDir,
		"SKILLHUB_ARTIFACT_DIR=" + ArtifactDir,
		"SKILLHUB_USER_PROMPT=" + req.TestCase.UserPrompt,
		"SKILLHUB_SKILL_CONTENT_HASH=" + req.SkillVersion.ContentHash,
		// The harness stamps this onto skill_activation events: the sandbox can
		// see a directory name but not which immutable version it came from, and
		// a trace that cannot name the version is not much use for improving it
		// (TRACE-002, iron rule 4).
		"SKILLHUB_SKILL_VERSION_ID=" + req.SkillVersion.SkillVersionID,
		"SKILLHUB_TRACE_LEVEL=" + req.Trace.Level,
		"SKILLHUB_ARTIFACT_MAX_BYTES=" + strconv.FormatInt(req.ResourceLimits.ArtifactFileBytes, 10),
		"HOME=" + WorkDir,
	}
	if req.Trace.IngestionURL != "" {
		e = append(e, "SKILLHUB_TRACE_URL="+req.Trace.IngestionURL)
	}
	// PDM-005 5.2a. The harness counts tokens and stops itself, because it is the
	// only party that sees a per-response count - RunUsage carries none back here
	// and the gateway's brakes are spend and rate, not tokens. These are the same
	// numbers the pre-run permission summary showed the user: they come off the
	// run's frozen policy_snapshot, which is the single object the summary, the
	// scheduler and this request are all read from (internal/run defaultPolicy).
	if tb := req.ResourceLimits.TokenBudget; tb != nil {
		if tb.MaxInputTokens > 0 {
			e = append(e, "SKILLHUB_MAX_INPUT_TOKENS="+strconv.FormatInt(tb.MaxInputTokens, 10))
		}
		if tb.MaxOutputTokens > 0 {
			e = append(e, "SKILLHUB_MAX_OUTPUT_TOKENS="+strconv.FormatInt(tb.MaxOutputTokens, 10))
		}
	}
	if req.Runtime.Model != "" {
		e = append(e, "SKILLHUB_MODEL="+req.Runtime.Model)
	}
	if g := req.ModelGateway; g != nil {
		e = append(e, "ANTHROPIC_BASE_URL="+g.BaseURL)
		if g.VirtualKey != "" {
			e = append(e, "ANTHROPIC_AUTH_TOKEN="+g.VirtualKey)
		}
	}
	return e
}

// tail reads back what the workload wrote. It is bounded and demultiplexed;
// the caller masks the injected secrets out of it before it goes anywhere.
func (d *Driver) tail(ctx context.Context, id string) string {
	rc, err := d.cli.ContainerLogs(ctx, name(id), container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: "200",
	})
	if err != nil {
		return ""
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, logTailBytes*2)); err != nil && buf.Len() == 0 {
		return ""
	}
	s := buf.String()
	if len(s) > logTailBytes {
		s = s[len(s)-logTailBytes:]
	}
	return strings.TrimSpace(s)
}

func devCmd(req sandbox.RunRequest) ([]string, bool) {
	raw, ok := req.Extensions["dev_cmd"].([]any)
	if !ok {
		return nil, false
	}
	cmd := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		cmd = append(cmd, s)
	}
	return cmd, len(cmd) > 0
}

// Rootless: New refuses a Config whose uid or gid is 0 (baseline C-02), and
// every container is created with User set from those two, so this is a
// restatement of a guarantee this driver already makes rather than a new
// check. It is a method rather than a constant because the Driver interface
// asks every driver the same question, and the other implementation's answer
// depends on the host.
func (d *Driver) Rootless() bool { return d.cfg.UID != 0 && d.cfg.GID != 0 }
