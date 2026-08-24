package sandbox

// SBX-008 / ADR-004 collectArtifacts: what the workload wrote, and where it went.
//
// The driver hands over one tar stream and never opens it. This file opens it,
// because the size ceilings of PDM-005 5.2 are the platform's to enforce and a
// workload cannot be asked to enforce a limit against itself: a run that writes
// a 4 GB file must be cut here, not trusted to have not written it.
//
// One archive, one object key. Pre-signing is per-object and the platform cannot
// know the file names before the workload produces them, so a single write grant
// can only authorize a single key (see grantsFor). The manifest is what names the
// individual files; their bytes are inside the archive at the grant's key.
//
// ponytail: one archive per attempt rather than one object per file. Reading a
// single artifact means fetching the archive. The upgrade path is a pre-signed
// POST policy scoped to a key prefix, which would let this upload each file to
// its own key - worth doing when the UI offers per-file download (PACK-001).

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	// uploadTimeout bounds one artifact upload. A stuck upload must not hold the
	// attempt open past its wall clock.
	uploadTimeout = 2 * time.Minute
	// artifactCollectTimeout bounds the whole read-filter-upload pass.
	artifactCollectTimeout = 5 * time.Minute
	// artifactMaxEntries bounds how many files one manifest can name. The byte
	// ceilings do not bound this at all - an empty file costs no bytes and still
	// costs a manifest entry, so a workload that touches 40k empty files makes a
	// RunResult the platform cannot read back (its provider response is cut at
	// 4 MiB) and loses its own run. 1000 entries is ~160 KB of manifest JSON,
	// two orders of magnitude inside that cap, and more files than a test run of
	// one skill has any reason to produce; the run's own bytes are still bounded
	// by ArtifactTotalBytes.
	artifactMaxEntries = 1000
)

// collect runs the collection handshake for one sandbox and reports whether it
// is finished with it.
//
// It is driven from the trace collector's tick because that is the only loop
// that runs while the workload is still alive, and being alive is the whole
// requirement: /out is a tmpfs and its contents are gone the moment the
// workload's process exits. When the workload says it has finished, this drains
// the last of the trace, collects the artifacts, and releases it.
//
// A workload that never says so - a crash, a kill, the wall clock - is never
// released and never waited for: this returns false, the loop keeps ticking, and
// Wait ends it. What was already drained is what survives.
func (m *Manager) collect(id, traceURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), artifactCollectTimeout)
	defer cancel()

	done, err := m.drv.WorkloadDone(ctx, id)
	if err != nil || !done {
		return false
	}
	m.mu.Lock()
	e := m.runs[id]
	m.mu.Unlock()
	if e == nil {
		return true // destroyed under us
	}

	artifacts := m.collectArtifacts(ctx, id, e)
	m.mu.Lock()
	e.artifacts = artifacts
	m.mu.Unlock()

	// The last drain happens after the artifacts, so an artifact failure is in
	// the trace before the workload is let go.
	m.flushTrace(id, traceURL)
	if err := m.drv.ReleaseWorkload(ctx, id); err != nil {
		m.log.Error("could not release the workload after collecting its output",
			"provider_run_id", id, "err", err)
	}
	return true
}

// collectArtifacts reads the workload's output out of the sandbox, enforces the
// per-run ceilings, uploads what survives and returns the manifest.
//
// Failure to collect is not failure of the run. The workload's own outcome is
// already decided by the time this runs, and losing the output of a run that
// otherwise succeeded is a smaller lie than reporting the run as failed - so the
// error is logged and the manifest comes back empty (ADR-004 keeps the run
// outcome and its side outcomes apart).
func (m *Manager) collectArtifacts(ctx context.Context, id string, e *entry) []Artifact {
	if e.artifactGrant == nil || e.artifactGrant.URL == "" {
		return nil // no write grant: nothing was authorized, so nothing is collected
	}
	raw, err := m.drv.ReadArtifacts(ctx, id)
	if err != nil {
		m.log.Warn("artifact collection failed", "provider_run_id", id, "err", err)
		return nil
	}
	if len(raw) == 0 {
		return nil
	}

	manifest, archive, err := filterArchive(raw, e.limits)
	if err != nil {
		m.log.Warn("artifact archive could not be read", "provider_run_id", id, "err", err)
		return nil
	}
	if len(manifest) == 0 {
		return nil
	}
	if err := upload(ctx, e.artifactGrant.URL, archive); err != nil {
		// The grant is the authorization and never reaches a log (iron rule 11).
		m.log.Error("artifact upload failed", "provider_run_id", id,
			"object_key", e.artifactGrant.ObjectKey, "err", err)
		return nil
	}
	m.log.Info("artifacts collected", "provider_run_id", id, "files", len(manifest))
	return manifest
}

// filterArchive turns the raw tar into a manifest plus the archive that is
// actually uploaded. Regular files only, each within the per-file ceiling, and
// the whole set within the per-run one; a file that breaks either is dropped
// from both, because an entry in the manifest whose bytes were not uploaded
// would be a manifest that lies.
//
// The count of entries is bounded as well as their bytes: the manifest is a
// JSON document the platform has to read back, and it is the one dimension a
// workload can inflate for free.
//
// `truncated` marks a run whose output was cut, so the UI never presents a
// partial collection as complete.
func filterArchive(raw []byte, limits ResourceLimits) ([]Artifact, []byte, error) {
	perFile := limits.ArtifactFileBytes
	if perFile <= 0 {
		perFile = DefaultLimits.ArtifactFileBytes
	}
	total := limits.ArtifactTotalBytes
	if total <= 0 {
		total = DefaultLimits.ArtifactTotalBytes
	}

	reader := tar.NewReader(bytes.NewReader(raw))
	var out bytes.Buffer
	writer := tar.NewWriter(&out)
	manifest := []Artifact{}
	var used int64
	dropped := false

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A truncated stream still carries whole entries before the cut. Keep
			// them and mark the collection as truncated rather than losing the lot.
			dropped = true
			break
		}
		if header.Typeflag != tar.TypeReg {
			continue // directories, links and devices carry no artifact bytes
		}
		if len(manifest) >= artifactMaxEntries {
			// Everything after this is dropped, and `truncated` is what says so.
			dropped = true
			break
		}
		name := artifactName(header.Name)
		if name == "" {
			dropped = true
			continue
		}
		if header.Size > perFile || used+header.Size > total {
			dropped = true
			// Skip the body without buffering it: the reader advances on Next().
			continue
		}
		body, err := io.ReadAll(io.LimitReader(reader, header.Size))
		if err != nil {
			dropped = true
			break
		}
		sum := sha256.Sum256(body)
		if err := writer.WriteHeader(&tar.Header{
			Name: name, Mode: 0o600, Size: int64(len(body)),
			ModTime: header.ModTime, Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, nil, err
		}
		if _, err := writer.Write(body); err != nil {
			return nil, nil, err
		}
		used += int64(len(body))
		manifest = append(manifest, Artifact{
			FileName:    name,
			SizeBytes:   int64(len(body)),
			ContentHash: hex.EncodeToString(sum[:]),
		})
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	if dropped {
		for i := range manifest {
			manifest[i].Truncated = true
		}
	}
	return manifest, out.Bytes(), nil
}

// artifactName reduces a tar entry to a name that cannot address anything
// outside the collection. The archive is untrusted output, so `../` and absolute
// paths are refused rather than cleaned into some other file's name.
func artifactName(raw string) string {
	name := strings.TrimPrefix(strings.ReplaceAll(raw, "\\", "/"), "artifacts/")
	name = strings.TrimPrefix(name, "./")
	switch {
	case name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "-"):
		return ""
	case name != path.Clean(name) || strings.HasPrefix(path.Clean(name), ".."):
		return ""
	}
	return name
}

// upload writes the archive to the object the write grant names. PUT is what a
// pre-signed upload URL authorizes: one object, one direction, until it expires.
func upload(ctx context.Context, url string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return errors.New("grant URL is not usable")
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("object storage could not be reached")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("object storage answered %d", resp.StatusCode)
	}
	return nil
}
