package dockerdrv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const (
	InputDir = WorkDir + "/.skillhub"

	ReadyPath = InputDir + "/ready"

	SkillArchivePath = InputDir + "/skill.zip"

	DatasetDir = WorkDir + "/data"

	ArtifactDir = OutDir + "/artifacts"

	DonePath      = OutDir + "/.workload-done"
	CollectedPath = OutDir + "/.collected"

	grantFetchLimit = 64 << 20

	grantFetchTimeout = 2 * time.Minute

	artifactReadLimit = 128 << 20
)

var errGone = errors.New("sandbox is no longer running")

func (d *Driver) pushInputs(ctx context.Context, id string, req sandbox.RunRequest) error {
	if err := d.exec(ctx, id, []string{"/bin/mkdir", "-p", InputDir, DatasetDir, ArtifactDir}, nil); err != nil {

		if errors.Is(err, errGone) || len(readGrants(req)) == 0 {
			return nil
		}
		return fmt.Errorf("prepare sandbox input directories: %w", err)
	}

	names := datasetNames(req)
	for _, g := range readGrants(req) {
		var target string
		switch g.Purpose {
		case "skill_package":
			target = SkillArchivePath
		case "dataset":
			name := names[g.ObjectKey]
			if name == "" {

				continue
			}
			target = DatasetDir + "/" + name
		default:
			continue
		}
		body, err := fetch(ctx, g.URL)
		if err != nil {

			return fmt.Errorf("fetch %s %s: %w", g.Purpose, g.ObjectKey, err)
		}
		if err := d.exec(ctx, id, []string{"/bin/dd", "of=" + target, "status=none"}, body); err != nil {
			if errors.Is(err, errGone) {
				return nil
			}
			return fmt.Errorf("place %s in the sandbox: %w", g.Purpose, err)
		}
	}

	if err := d.exec(ctx, id, []string{"/bin/dd", "of=" + ReadyPath, "status=none"}, []byte("ready\n")); err != nil {

		slog.Warn("could not signal the sandbox that its inputs are ready",
			"provider_run_id", id, "err", err)
	}
	return nil
}

func readGrants(req sandbox.RunRequest) []sandbox.ObjectGrant {
	out := make([]sandbox.ObjectGrant, 0, len(req.ObjectGrants))
	for _, g := range req.ObjectGrants {
		if g.Access == "read" && g.URL != "" &&
			(g.Purpose == "skill_package" || g.Purpose == "dataset") {
			out = append(out, g)
		}
	}
	return out
}

func datasetNames(req sandbox.RunRequest) map[string]string {
	out := map[string]string{}
	for _, ref := range req.TestCase.DatasetRefs {
		if ref.ObjectKey == "" {
			continue
		}
		name := path.Base(strings.ReplaceAll(ref.FileName, "\\", "/"))
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") {
			continue
		}
		out[ref.ObjectKey] = name
	}
	return out
}

func (d *Driver) ReadArtifacts(ctx context.Context, id string) ([]byte, error) {
	var out bytes.Buffer

	err := d.execOut(ctx, id, []string{"/bin/tar", "-cf", "-", "-C", OutDir, "artifacts"}, &out, artifactReadLimit)
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (d *Driver) WorkloadDone(ctx context.Context, id string) (bool, error) {
	var out bytes.Buffer
	if err := d.execOut(ctx, id, []string{"/bin/cat", DonePath}, &out, 1<<10); err != nil {
		return false, err
	}
	return out.Len() > 0, nil
}

func (d *Driver) ReleaseWorkload(ctx context.Context, id string) error {
	err := d.exec(ctx, id, []string{"/bin/dd", "of=" + CollectedPath, "status=none"}, []byte("collected\n"))
	if errors.Is(err, errGone) {
		return nil
	}
	return err
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	return fetchWithLimit(ctx, url, grantFetchLimit)
}

func fetchWithLimit(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, grantFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("grant URL is not usable")
	}
	resp, err := sandbox.GrantHTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("object storage could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("object storage answered %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("object could not be read")
	}
	if int64(len(body)) > limit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	return body, nil
}

func (d *Driver) exec(ctx context.Context, id string, cmd []string, stdin []byte) error {
	err := d.execOnce(ctx, id, cmd, stdin)
	if err != nil && !d.isRunning(ctx, id) {
		return errGone
	}
	return err
}

func (d *Driver) isRunning(ctx context.Context, id string) bool {
	insp, err := d.cli.ContainerInspect(context.WithoutCancel(ctx), name(id), client.ContainerInspectOptions{})
	if err != nil {
		return !cerrdefs.IsNotFound(err)
	}
	return insp.Container.State != nil && insp.Container.State.Running
}

func (d *Driver) execOnce(ctx context.Context, id string, cmd []string, stdin []byte) error {
	created, err := d.cli.ExecCreate(ctx, name(id), client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		return err
	}
	attached, err := d.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()

	if stdin != nil {
		if _, err := attached.Conn.Write(stdin); err != nil {
			return err
		}

		if err := attached.CloseWrite(); err != nil {
			return err
		}
	}

	if _, err := stdcopy.StdCopy(io.Discard, io.Discard, io.LimitReader(attached.Reader, 1<<20)); err != nil {
		return err
	}
	inspect, err := d.cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspect.Running {
		return fmt.Errorf("%s was still running after its streams closed", cmd[0])
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("%s exited with code %d", cmd[0], inspect.ExitCode)
	}
	return nil
}

func (d *Driver) execOut(ctx context.Context, id string, cmd []string, out io.Writer, limit int64) error {
	created, err := d.cli.ExecCreate(ctx, name(id), client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			return nil
		}
		return err
	}
	attached, err := d.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()
	if _, err := stdcopy.StdCopy(out, io.Discard, io.LimitReader(attached.Reader, limit)); err != nil {
		return nil //nolint:nilerr // a truncated stream is bounded output, not a failure to report
	}
	return nil
}
