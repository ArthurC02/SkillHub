package dockerdrv

// SBX-008: moving bytes in and out of a sandbox that has no route to object
// storage.
//
// The sandbox's only network is the egress one, and object storage is not on it
// (SBX-007). So the execution node fetches the run's inputs with the short-lived
// grants it was dispatched with, writes them into the container, and reads the
// collected output back out the same way. The node holds a URL for the duration
// of one run and nothing else - no storage credential, no database connection
// (iron rule 2).
//
// Both directions go through `docker exec`, and that is forced rather than
// preferred: `docker cp` is refused outright by the daemon for a container with
// a read-only rootfs ("container rootfs is marked read-only"), which every
// sandbox has (C-06), and it resolves paths against the rootfs layer anyway so
// it cannot see under the /work and /out tmpfs mounts. A bind mount would put a
// host path inside the sandbox, which C-05 forbids. The risk boundary of exec is
// the one ReadTrace already argues: image-owned binaries on a read-only rootfs,
// absolute paths, no shell, no caller-supplied arguments, the unprivileged uid
// the workload already runs as.
//
// The node never opens what it moves. An archive is written whole and untarred
// by the workload; a dataset is written whole and read by the workload. Parsing
// untrusted bytes in this process is what iron rule 1 forbids.

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
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const (
	// InputDir holds what the node put there before the workload started. It is
	// under /work because that tmpfs is the only writable place a sandbox has.
	InputDir = WorkDir + "/.skillhub"
	// ReadyPath is the handshake. The workload blocks until this file exists, so
	// "the container started" and "its inputs are there" are not the same
	// instant and nothing has to guess (see run.mjs).
	ReadyPath = InputDir + "/ready"
	// SkillArchivePath is where the skill package lands, unopened. The archive is
	// the zip that ingest stored (packages/<hash>.zip); the node moves the bytes
	// and the sandbox is what unpacks them (iron rule 1).
	SkillArchivePath = InputDir + "/skill.zip"
	// DatasetDir holds the test case's frozen dataset files, one per name.
	DatasetDir = WorkDir + "/data"
	// ArtifactDir is what the workload writes and the node collects.
	ArtifactDir = OutDir + "/artifacts"
	// DonePath and CollectedPath are the collection handshake, and it exists for
	// a reason that is not negotiable: /out is a tmpfs, so the instant the
	// workload's process exits the kernel discards everything in it - the tail of
	// the trace and every artifact with it. `docker exec` also needs a *running*
	// container, and `docker cp` is refused on a read-only rootfs and cannot see
	// under a mount anyway, so there is no post-exit read of any kind.
	//
	// So the workload announces that it has finished (DonePath) and waits; the
	// node drains the trace, collects the artifacts, and then releases it
	// (CollectedPath). This is PDM-005's cooperative window written down: the
	// gap between "the work is done" and "the sandbox is gone" is where the
	// output is taken out.
	//
	// A workload that dies without writing DonePath - a crash, a kill, the wall
	// clock - loses whatever the live 2-second drain had not already carried out.
	// That is a real limit of a tmpfs scratch space, not something a handshake
	// can fix.
	DonePath      = OutDir + "/.workload-done"
	CollectedPath = OutDir + "/.collected"

	// grantFetchLimit bounds one download from object storage. PDM-005 5.1 caps a
	// single dataset at 25 MB and a package is smaller again; this is the second
	// bound, applied to bytes about to enter this process.
	grantFetchLimit = 64 << 20
	// grantFetchTimeout bounds one download. A stuck fetch must not hold a run
	// slot until the wall clock expires.
	grantFetchTimeout = 2 * time.Minute
	// artifactReadLimit bounds one collection out of a sandbox. The per-run
	// ceiling from PDM-005 5.2 is applied on top of it by the caller.
	artifactReadLimit = 128 << 20
)

// errGone is a sandbox that is no longer running. Its workload finished on its
// own terms before the inputs were placed, which is not an input failure: Wait
// is about to report whatever it actually did, and blaming the delivery would
// replace the real outcome with a misleading one.
var errGone = errors.New("sandbox is no longer running")

// pushInputs fetches the run's inputs and writes them into the sandbox, then
// drops the ready marker. Order matters: the marker is last, so a workload that
// sees it sees everything.
//
// What is fatal here is narrow on purpose: **a granted object that could not be
// placed**, because that is the case where the run would go on to execute
// without something it was given. The scaffolding around it - the directories
// and the ready marker - is best effort, because a workload that does not take
// part in the handshake is a workload that has nothing to be withheld from. The
// isolation probes are exactly that: they replace the image's entrypoint, carry
// no grants, and are frequently gone before the first exec can run.
func (d *Driver) pushInputs(ctx context.Context, id string, req sandbox.RunRequest) error {
	if err := d.exec(ctx, id, []string{"/bin/mkdir", "-p", InputDir, DatasetDir, ArtifactDir}, nil); err != nil {
		// Nothing can be placed, so nothing further will succeed either. A run
		// that was granted something says so; one that was not simply has no
		// inputs and no marker, which is the state it would have had anyway.
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
				// A grant for a file this test case snapshot does not name. Not
				// this node's to interpret: it is dropped rather than written
				// under a guessed name.
				continue
			}
			target = DatasetDir + "/" + name
		default:
			continue
		}
		body, err := fetch(ctx, g.URL)
		if err != nil {
			// The URL is the authorization, so it must not reach the message
			// (iron rule 11) - the purpose and the object key are enough to
			// diagnose with, and neither is secret.
			return fmt.Errorf("fetch %s %s: %w", g.Purpose, g.ObjectKey, err)
		}
		if err := d.exec(ctx, id, []string{"/bin/dd", "of=" + target, "status=none"}, body); err != nil {
			if errors.Is(err, errGone) {
				return nil
			}
			return fmt.Errorf("place %s in the sandbox: %w", g.Purpose, err)
		}
	}
	// Best effort, and last: a workload that never sees this reports the missing
	// inputs itself, in its own trace, which is a better diagnosis than this
	// process guessing why the write did not land.
	if err := d.exec(ctx, id, []string{"/bin/dd", "of=" + ReadyPath, "status=none"}, []byte("ready\n")); err != nil {
		// Identifiers and the failure only: a RunRequest carries grant URLs and a
		// Virtual Key, and none of it belongs in a log line (iron rule 11).
		slog.Warn("could not signal the sandbox that its inputs are ready",
			"provider_run_id", id, "err", err)
	}
	return nil
}

// readGrants are the authorizations that move bytes *into* the sandbox: the ones
// with a URL to fetch and read access to use it. artifact_upload is a write
// grant and goes the other way.
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

// datasetNames maps each granted object onto the file name the frozen test case
// snapshot gave it. The name is user-supplied, so it is reduced to a base name
// and anything that could still escape /work/data is refused (a grant with an
// unusable name is dropped, not sanitised into a different file).
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

// ReadArtifacts returns the workload's collected output as a tar stream, or
// nothing when it wrote none. The bytes are untrusted and are not opened here;
// the manager parses them to build the manifest and to enforce the size
// ceilings.
//
// A missing directory, a stopped container and an unreadable stream are all
// "nothing collected", the same as ReadTrace: an empty /out/artifacts is the
// ordinary case, not an error.
func (d *Driver) ReadArtifacts(ctx context.Context, id string) ([]byte, error) {
	var out bytes.Buffer
	// `-C /out artifacts` and not `/out/artifacts` so the archive carries
	// relative paths; a tar of absolute paths is a tar nobody should unpack.
	err := d.execOut(ctx, id, []string{"/bin/tar", "-cf", "-", "-C", OutDir, "artifacts"}, &out, artifactReadLimit)
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// WorkloadDone reports whether the workload has finished its own work and is
// waiting to be released. A sandbox that is already gone answers false: there is
// nothing left to collect from it either way.
func (d *Driver) WorkloadDone(ctx context.Context, id string) (bool, error) {
	var out bytes.Buffer
	if err := d.execOut(ctx, id, []string{"/bin/cat", DonePath}, &out, 1<<10); err != nil {
		return false, err
	}
	return out.Len() > 0, nil
}

// ReleaseWorkload tells a waiting workload that its output has been taken and it
// may exit. Best effort: a workload that is no longer waiting has already left,
// which is the state this call was asking for.
func (d *Driver) ReleaseWorkload(ctx context.Context, id string) error {
	err := d.exec(ctx, id, []string{"/bin/dd", "of=" + CollectedPath, "status=none"}, []byte("collected\n"))
	if errors.Is(err, errGone) {
		return nil
	}
	return err
}

// fetch downloads one granted object. Bounded in size and in time, and the URL
// never appears in an error.
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

// exec runs one image-owned binary in the sandbox, optionally feeding it stdin,
// and reports whether it succeeded. Output is discarded: the callers here place
// files, they do not read them.
//
// A failure is re-judged against the container itself, because "the workload
// exited while this exec was in flight" reaches the caller in a different shape
// depending on how far it got, and all three are the same fact:
//
//	ContainerExecCreate   409 "container ... is not running"
//	ContainerExecAttach   "unable to upgrade to tcp, received 404"
//	ContainerExecInspect  ExitCode 128, with no process ever started
//
// All three were reproduced against a real daemon. Matching on their shapes
// would be matching on daemon prose; asking whether the container is still
// running answers the actual question and does not go stale.
func (d *Driver) exec(ctx context.Context, id string, cmd []string, stdin []byte) error {
	err := d.execOnce(ctx, id, cmd, stdin)
	if err != nil && !d.isRunning(ctx, id) {
		return errGone
	}
	return err
}

// isRunning answers whether the sandbox is still there to be written to. An
// unreachable daemon answers "yes", so a lost connection is reported as the
// error it is rather than disguised as a finished workload.
func (d *Driver) isRunning(ctx context.Context, id string) bool {
	insp, err := d.cli.ContainerInspect(context.WithoutCancel(ctx), name(id))
	if err != nil {
		return !cerrdefs.IsNotFound(err)
	}
	return insp.State != nil && insp.State.Running
}

func (d *Driver) execOnce(ctx context.Context, id string, cmd []string, stdin []byte) error {
	created, err := d.cli.ContainerExecCreate(ctx, name(id), container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		return err
	}
	attached, err := d.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()

	if stdin != nil {
		if _, err := attached.Conn.Write(stdin); err != nil {
			return err
		}
		// Without this the command never sees EOF and waits forever.
		if err := attached.CloseWrite(); err != nil {
			return err
		}
	}
	// Drain before inspecting: the exec is only finished once its streams are.
	if _, err := stdcopy.StdCopy(io.Discard, io.Discard, io.LimitReader(attached.Reader, 1<<20)); err != nil {
		return err
	}
	inspect, err := d.cli.ContainerExecInspect(ctx, created.ID)
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

// execOut runs one image-owned binary and collects its stdout, bounded. stderr
// is discarded, so "no such directory" reads as nothing collected rather than as
// corrupt output.
func (d *Driver) execOut(ctx context.Context, id string, cmd []string, out io.Writer, limit int64) error {
	created, err := d.cli.ContainerExecCreate(ctx, name(id), container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		User:         fmt.Sprintf("%d:%d", d.cfg.UID, d.cfg.GID),
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			return nil // gone, or not running
		}
		return err
	}
	attached, err := d.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()
	if _, err := stdcopy.StdCopy(out, io.Discard, io.LimitReader(attached.Reader, limit)); err != nil {
		return nil //nolint:nilerr // a truncated stream is bounded output, not a failure to report
	}
	return nil
}
