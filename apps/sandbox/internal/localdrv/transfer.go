package localdrv

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

	grantFetchLimit   = 64 << 20
	grantFetchTimeout = 2 * time.Minute

	artifactReadLimit = 128 << 20

	traceReadLimit = 8 << 20
)

func inputDir(workDir string) string     { return filepath.Join(workDir, inputSubdir) }
func datasetDir(workDir string) string   { return filepath.Join(workDir, datasetSubdir) }
func artifactDir(outDir string) string   { return filepath.Join(outDir, artifactSubdir) }
func tracePath(outDir string) string     { return filepath.Join(outDir, traceSubdir, traceName) }
func donePath(outDir string) string      { return filepath.Join(outDir, doneName) }
func collectedPath(outDir string) string { return filepath.Join(outDir, collectedName) }

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

				continue
			}
			target = filepath.Join(datasetDir(r.workDir), name)
		default:
			continue
		}
		body, err := fetch(ctx, g.URL)
		if err != nil {

			return fmt.Errorf("fetch %s %s: %w", g.Purpose, g.ObjectKey, err)
		}
		if err := os.WriteFile(target, body, 0o600); err != nil {
			return fmt.Errorf("place %s in the run directory: %w", g.Purpose, err)
		}
	}
	_ = os.WriteFile(filepath.Join(inputDir(r.workDir), readyName), []byte("ready\n"), 0o600)
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
		name := filepath.Base(strings.ReplaceAll(ref.FileName, "\\", "/"))
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") {
			continue
		}
		out[ref.ObjectKey] = name
	}
	return out
}

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

func (d *Driver) ReadArtifacts(ctx context.Context, id string) ([]byte, error) {
	r := d.get(id)
	if r == nil {
		return nil, nil
	}
	dir := artifactDir(r.outDir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
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

		rel = filepath.ToSlash(rel)

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

func (d *Driver) WorkloadDone(ctx context.Context, id string) (bool, error) {
	r := d.get(id)
	if r == nil {
		return false, nil
	}
	_, err := os.Stat(donePath(r.outDir))
	return err == nil, nil
}

func (d *Driver) ReleaseWorkload(ctx context.Context, id string) error {
	r := d.get(id)
	if r == nil {
		return nil
	}
	err := os.WriteFile(collectedPath(r.outDir), []byte("collected\n"), 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
