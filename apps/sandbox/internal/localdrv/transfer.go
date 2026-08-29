package localdrv

// Moving bytes in and out of a run directory that is just a directory: the host
// process already has whatever network route this node has (there is no
// sandbox network to be missing, unlike dockerdrv's SBX-007 story), so this is
// simpler than dockerdrv/transfer.go in exactly the ways the container
// boundary made hard there. No `docker exec`, no read-only-rootfs refusal of
// `docker cp`, no risk-bounded exec into untrusted code to move a file — this
// writes straight to paths under the run's own directory.
//
// What does not change is the collection handshake itself (PDM-005's
// cooperative stop window): run.mjs is unmodified, so it still announces
// DonePath and waits for CollectedPath before it lets its own process exit.
// The only thing that changed is that the files it is waiting on now live on
// the host filesystem instead of a container's tmpfs — which means, unlike
// dockerdrv, they do not vanish the instant the workload's process exits. The
// handshake is followed anyway, because Manager (sandbox/artifacts.go) is
// written against the Driver interface, not against this driver's filesystem,
// and diverging from it here would be a second, undocumented dispatch path.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const (
	inputSubdir    = ".skillhub"
	readyName      = "ready"
	archiveName    = "skill.zip"
	datasetSubdir  = "data"
	artifactSubdir = "artifacts"
	traceSubdir    = "trace"
	traceName      = "events.jsonl"
	doneName       = ".workload-done"
	collectedName  = ".collected"

	// grantFetchLimit and grantFetchTimeout mirror dockerdrv/transfer.go's own
	// bounds (PDM-005 5.1): the second bound applied to bytes about to enter
	// this process, on top of whatever object storage itself enforces.
	grantFetchLimit   = 64 << 20
	grantFetchTimeout = 2 * time.Minute
	// artifactReadLimit bounds one collection out of a run directory, the same
	// role dockerdrv's own constant plays before the manager applies the run's
	// actual ceilings on top.
	artifactReadLimit = 128 << 20
	// traceReadLimit bounds one read of the trace file.
	traceReadLimit = 8 << 20
)

func inputDir(workDir string) string     { return filepath.Join(workDir, inputSubdir) }
func datasetDir(workDir string) string   { return filepath.Join(workDir, datasetSubdir) }
func artifactDir(outDir string) string   { return filepath.Join(outDir, artifactSubdir) }
func tracePath(outDir string) string     { return filepath.Join(outDir, traceSubdir, traceName) }
func donePath(outDir string) string      { return filepath.Join(outDir, doneName) }
func collectedPath(outDir string) string { return filepath.Join(outDir, collectedName) }

// pushInputs fetches the run's granted inputs and writes them into the run
// directory, then drops the ready marker last — the same ordering dockerdrv
// uses and for the same reason: a workload that sees the marker sees
// everything.
//
// What is fatal here is narrow, matching dockerdrv/transfer.go's own pushInputs:
// a granted object that could not be placed, because that is the one case
// where the run would go on to execute without something it was given. The
// ready marker itself is best effort — a workload that never sees it has
// nothing to be withheld from.
func (d *Driver) pushInputs(ctx context.Context, r *run, req sandbox.RunRequest) error {
	names := datasetNames(req)
	for _, g := range readGrants(req) {
		var target string
		switch g.Purpose {
		case "skill_package":
			target = filepath.Join(inputDir(r.workDir), archiveName)
		case "dataset":
			name := names[g.ObjectKey]
			if name == "" {
				// A grant for a file this test case snapshot does not name.
				// Not this driver's to interpret: dropped rather than written
				// under a guessed name.
				continue
			}
			target = filepath.Join(datasetDir(r.workDir), name)
		default:
			continue
		}
		body, err := fetch(ctx, g.URL)
		if err != nil {
			// The URL is the authorization, so it must not reach the message
			// (iron rule 11) — purpose and object key are enough to diagnose
			// with and neither is secret.
			return fmt.Errorf("fetch %s %s: %w", g.Purpose, g.ObjectKey, err)
		}
		if err := os.WriteFile(target, body, 0o600); err != nil {
			return fmt.Errorf("place %s in the run directory: %w", g.Purpose, err)
		}
	}
	_ = os.WriteFile(filepath.Join(inputDir(r.workDir), readyName), []byte("ready\n"), 0o600)
	return nil
}

// readGrants are the authorizations that move bytes *into* the run: the ones
// with a URL to fetch and read access to use it. artifact_upload is a write
// grant to object storage and is not this driver's concern — Manager reads
// artifacts back through ReadArtifacts and uploads them itself.
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

// datasetNames maps each granted object onto the file name the frozen test
// case snapshot gave it. The name is user-supplied, so it is reduced to a base
// name and anything that could still escape the dataset directory is refused —
// a grant with an unusable name is dropped, not sanitised into a different
// file.
func datasetNames(req sandbox.RunRequest) map[string]string {
	out := map[string]string{}
	for _, ref := range req.TestCase.DatasetRefs {
		if ref.ObjectKey == "" {
			continue
		}
		name := filepath.Base(strings.ReplaceAll(ref.FileName, "\\", "/"))
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") {
			continue
		}
		out[ref.ObjectKey] = name
	}
	return out
}

// fetch downloads one granted object, bounded in size and in time. The URL
// never appears in an error (iron rule 11).
func fetch(ctx context.Context, url string) ([]byte, error) {
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
	if resp.ContentLength > grantFetchLimit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, grantFetchLimit+1))
	if err != nil {
		return nil, errors.New("object could not be read")
	}
	if int64(len(body)) > grantFetchLimit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	return body, nil
}

// limitWriter refuses to accept more than limit bytes total, so ReadArtifacts
// has its own bound on what enters this process before the manager applies the
// run's actual ceilings on top (iron rule 1) — the same second-bound role
// dockerdrv's artifactReadLimit plays via a limited exec read.
type limitWriter struct {
	w     io.Writer
	limit int64
	n     int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n+int64(len(p)) > l.limit {
		return 0, errors.New("artifact archive exceeds the read bound")
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

// ReadArtifacts returns the workload's collected output as a tar stream, or
// nothing when it wrote none. Like dockerdrv, the bytes are untrusted and are
// not opened here; the manager parses them to build the manifest and enforce
// the size ceilings.
func (d *Driver) ReadArtifacts(ctx context.Context, id string) ([]byte, error) {
	r := d.get(id)
	if r == nil {
		return nil, nil // gone: "nothing collected", same as dockerdrv
	}
	dir := artifactDir(r.outDir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil // an empty/missing artifacts directory is the ordinary case
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&limitWriter{w: &buf, limit: artifactReadLimit})
	walkErr := filepath.WalkDir(dir, func(path string, de fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		// Relative, forward-slash paths only: a tar of absolute paths is a tar
		// nobody should unpack, and dockerdrv makes the same choice with
		// `-C /out artifacts`.
		rel = filepath.ToSlash(rel)
		// Symlinks, fifos, sockets and devices are skipped, not refused. Both
		// halves of that matter and neither used to hold:
		//
		//   - tar.FileInfoHeader builds a symlink header whose Size is the
		//     length of the link *target string*, and the os.Open below follows
		//     the link and copies the pointed-at file's contents instead. The
		//     two disagree, tar.Writer returns ErrWriteTooLong, and the whole
		//     collection fails — so one stray link cost the run every artifact
		//     it produced while the run itself still reported success.
		//   - os.Open on a POSIX fifo blocks until somebody opens the other
		//     end, which nobody does, so collection burned its whole timeout.
		//
		// dockerdrv already behaves this way: /bin/tar stores a symlink as a
		// symlink and filterArchive drops every entry that is not TypeReg. Two
		// drivers behind one Driver interface must not turn "one unusable file"
		// into "a file missing" on one and "nothing collected" on the other.
		if !de.IsDir() && !de.Type().IsRegular() {
			return nil
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if de.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, nil
	}
	return buf.Bytes(), nil
}

// ReadTrace returns a bounded JSONL chunk beginning at byte offset from the
// run's trace file. An absent file is not an error: it means the workload has
// not emitted anything yet.
func (d *Driver) ReadTrace(ctx context.Context, id string, offset int64) ([]byte, bool, error) {
	r := d.get(id)
	if r == nil {
		return nil, false, nil
	}
	f, err := os.Open(tracePath(r.outDir))
	if err != nil {
		return nil, false, nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || offset >= info.Size() {
		return nil, false, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(f, traceReadLimit+1))
	if err != nil {
		return nil, false, err
	}
	more := int64(len(data)) > traceReadLimit
	if more {
		data = data[:traceReadLimit]
	}
	return data, more, nil
}

// WorkloadDone reports whether the workload has finished its own work and is
// waiting to be released. A sandbox that is already gone answers false: there
// is nothing left to collect from it either way.
func (d *Driver) WorkloadDone(ctx context.Context, id string) (bool, error) {
	r := d.get(id)
	if r == nil {
		return false, nil
	}
	_, err := os.Stat(donePath(r.outDir))
	return err == nil, nil
}

// ReleaseWorkload tells a waiting workload that its output has been taken and
// it may exit. Best effort: a workload that is no longer waiting has already
// left, which is the state this call was asking for.
func (d *Driver) ReleaseWorkload(ctx context.Context, id string) error {
	r := d.get(id)
	if r == nil {
		return nil
	}
	err := os.WriteFile(collectedPath(r.outDir), []byte("collected\n"), 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil // the run directory is gone; nothing waiting to be released
	}
	return err
}
