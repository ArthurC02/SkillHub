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
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const (
	WorkDir = "/work"
	OutDir  = "/out"

	labelManaged   = "skillhub.sandbox.managed"
	labelRunID     = "skillhub.sandbox.run_id"
	labelAttemptID = "skillhub.sandbox.run_attempt_id"
	labelAttempt   = "skillhub.sandbox.attempt"
	labelWorkspace = "skillhub.sandbox.workspace_id"
	labelHandle    = "skillhub.sandbox.provider_run_id"

	labelProbe    = "skillhub.sandbox.probe"
	labelHash     = "skillhub.sandbox.request_hash"
	labelDeadline = "skillhub.sandbox.hard_deadline"

	logTailBytes = 32 << 10

	TracePath = OutDir + "/trace/events.jsonl"

	traceReadLimit = 8 << 20
)

type Config struct {
	Image string

	Runtime string

	Network string

	UID, GID int

	StorageQuota bool

	AllowDevCmd bool

	ExtraLabels map[string]string
}

type Driver struct {
	cli *client.Client
	cfg Config
}

func New(cfg Config) (*Driver, error) {
	cli, err := client.New(client.FromEnv)
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

func (d *Driver) Start(ctx context.Context, id string, req sandbox.RunRequest) error {
	lim := req.ResourceLimits

	workBytes := lim.DiskBytes * 3 / 4
	outBytes := lim.DiskBytes - workBytes
	mount := func(size int64, extra string) string {
		return fmt.Sprintf("rw,nosuid,nodev,size=%d,uid=%d,gid=%d,mode=0700%s", size, d.cfg.UID, d.cfg.GID, extra)
	}

	network := d.networkFor(req)

	cfg := &container.Config{
		Image:      d.cfg.Image,
		User:       fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
		Env:        env(req),
		WorkingDir: WorkDir,
		Labels:     d.labels(req, id),

		Tty:             false,
		OpenStdin:       false,
		NetworkDisabled: network == "none",
	}
	if cmd, ok := devCmd(req); ok && d.cfg.AllowDevCmd {
		cfg.Cmd = cmd
	}

	pids := lim.MaxPIDs
	hc := &container.HostConfig{

		ReadonlyRootfs: true,
		Tmpfs: map[string]string{
			WorkDir: mount(workBytes, ""),
			OutDir:  mount(outBytes, ""),

			"/tmp": mount(64<<20, ",noexec"),
		},

		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Privileged:  false,

		NetworkMode: container.NetworkMode(network),
		Runtime:     d.cfg.Runtime,
		AutoRemove:  false,
		Resources: container.Resources{
			NanoCPUs:   int64(lim.VCPU * 1e9),
			Memory:     lim.MemoryBytes,
			MemorySwap: lim.MemoryBytes,
			PidsLimit:  &pids,
			Ulimits: []*container.Ulimit{
				{Name: "nofile", Soft: lim.MaxOpenFiles, Hard: lim.MaxOpenFiles},

				{Name: "core", Soft: 0, Hard: 0},
			},
		},

		LogConfig: container.LogConfig{
			Type:   "json-file",
			Config: map[string]string{"max-size": "16m", "max-file": "1"},
		},
	}
	if d.cfg.StorageQuota {
		hc.StorageOpt = map[string]string{"size": strconv.FormatInt(lim.DiskBytes, 10)}
	}

	created, err := d.cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: cfg, HostConfig: hc, Name: name(id)})
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}
	if _, err := d.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {

		return fmt.Errorf("start sandbox: %w", err)
	}

	return d.pushInputs(ctx, id, req)
}

func (d *Driver) networkFor(req sandbox.RunRequest) string {
	if d.cfg.Network == "" || d.cfg.Network == "none" {
		return "none"
	}

	if len(req.Egress.Allow) > 0 {
		return d.cfg.Network
	}
	return "none"
}

func (d *Driver) Wait(ctx context.Context, id string) (sandbox.Outcome, error) {
	wait := d.cli.ContainerWait(ctx, name(id), client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-wait.Error:
		return sandbox.Outcome{}, err
	case <-ctx.Done():
		return sandbox.Outcome{}, ctx.Err()
	case st := <-wait.Result:
		out := sandbox.Outcome{ExitCode: int(st.StatusCode)}
		if st.Error != nil {
			return out, errors.New(st.Error.Message)
		}

		if insp, err := d.cli.ContainerInspect(context.WithoutCancel(ctx), name(id), client.ContainerInspectOptions{}); err == nil && insp.Container.State != nil {
			out.OOMKilled = insp.Container.State.OOMKilled
		}
		out.Output = d.tail(context.WithoutCancel(ctx), id)
		return out, nil
	}
}

func (d *Driver) Stop(ctx context.Context, id string, grace time.Duration) error {
	secs := int(grace.Seconds())
	if secs < 0 {
		secs = 0
	}
	_, err := d.cli.ContainerStop(ctx, name(id), client.ContainerStopOptions{Timeout: &secs})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *Driver) Remove(ctx context.Context, id string) error {
	_, err := d.cli.ContainerRemove(ctx, name(id), client.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *Driver) Adopt(ctx context.Context) ([]sandbox.Adopted, error) {
	list, err := d.cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", labelManaged+"=1"),
	})
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Adopted, 0, len(list.Items))
	for _, c := range list.Items {
		attempt, _ := strconv.Atoi(c.Labels[labelAttempt])
		deadline, _ := time.Parse(time.RFC3339, c.Labels[labelDeadline])
		out = append(out, sandbox.Adopted{
			ProviderRunID: c.Labels[labelHandle],
			RunID:         c.Labels[labelRunID],
			RunAttemptID:  c.Labels[labelAttemptID],
			Attempt:       attempt,
			RequestHash:   c.Labels[labelHash],
			CreatedAt:     time.Unix(c.Created, 0).UTC(),
			Running:       c.State == container.StateRunning,
			HardDeadline:  deadline,
		})
	}
	return out, nil
}

// dd only seeks in whole blocks, so this reads from the block containing
// offset and trims the leading bytes before it in Go.
func (d *Driver) ReadTrace(ctx context.Context, id string, offset int64) ([]byte, bool, error) {
	const blockSize = int64(1 << 20)
	base := offset / blockSize * blockSize
	prefix := int(offset - base)
	exec, err := d.cli.ExecCreate(ctx, name(id), client.ExecCreateOptions{
		Cmd: []string{
			"/bin/dd", "if=" + TracePath, "bs=" + strconv.FormatInt(blockSize, 10),
			"skip=" + strconv.FormatInt(base/blockSize, 10), "count=9",
		},
		AttachStdout: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	attached, err := d.cli.ExecAttach(ctx, exec.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, false, err
	}
	defer attached.Close()

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
	_, err := d.cli.Ping(ctx, client.PingOptions{})
	return err == nil
}

func (d *Driver) labels(req sandbox.RunRequest, id string) map[string]string {
	l := map[string]string{
		labelManaged:   "1",
		labelHandle:    id,
		labelRunID:     req.RunID,
		labelAttemptID: req.RunAttemptID,
		labelAttempt:   strconv.Itoa(req.Attempt),

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

		"SKILLHUB_SKILL_VERSION_ID=" + req.SkillVersion.SkillVersionID,
		"SKILLHUB_TRACE_LEVEL=" + req.Trace.Level,
		"SKILLHUB_ARTIFACT_MAX_BYTES=" + strconv.FormatInt(req.ResourceLimits.ArtifactFileBytes, 10),
		"HOME=" + WorkDir,
	}
	if req.Trace.IngestionURL != "" {
		e = append(e, "SKILLHUB_TRACE_URL="+req.Trace.IngestionURL)
	}

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

func (d *Driver) tail(ctx context.Context, id string) string {
	rc, err := d.cli.ContainerLogs(ctx, name(id), client.ContainerLogsOptions{
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

func (d *Driver) Rootless() bool { return d.cfg.UID != 0 && d.cfg.GID != 0 }
