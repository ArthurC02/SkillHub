package sandbox

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
	"unicode/utf8"
)

const (
	uploadTimeout = 2 * time.Minute

	artifactCollectTimeout = 5 * time.Minute

	artifactMaxEntries = 1000
)

func (m *Manager) collect(parent context.Context, id, traceURL string) bool {
	ctx, cancel := context.WithTimeout(parent, artifactCollectTimeout)
	defer cancel()

	done, err := m.drv.WorkloadDone(ctx, id)
	if err != nil || !done {
		return false
	}
	m.mu.Lock()
	e := m.runs[id]
	m.mu.Unlock()
	if e == nil {
		return true
	}

	artifacts, truncated := m.collectArtifacts(ctx, id, e)
	m.mu.Lock()

	if len(artifacts) > 0 || len(e.artifacts) == 0 {
		e.artifacts = artifacts
	}
	e.artifactsTruncated = e.artifactsTruncated || truncated
	m.mu.Unlock()

	if !m.flushTrace(ctx, id, traceURL) {
		return false
	}
	if err := m.drv.ReleaseWorkload(ctx, id); err != nil {
		m.log.Error("could not release the workload after collecting its output",
			"provider_run_id", id, "err", err)
		return false
	}
	return true
}

func (m *Manager) collectArtifacts(ctx context.Context, id string, e *entry) ([]Artifact, bool) {
	if e.artifactGrant == nil || e.artifactGrant.URL == "" {
		return nil, false
	}
	raw, err := m.drv.ReadArtifacts(ctx, id)
	if err != nil {
		m.log.Warn("artifact collection failed", "provider_run_id", id, "err", err)
		return nil, false
	}
	if len(raw) == 0 {
		return nil, false
	}

	manifest, archive, truncated, err := filterArchive(raw, e.limits)
	if err != nil {
		m.log.Warn("artifact archive could not be read", "provider_run_id", id, "err", err)
		return nil, true
	}
	if len(manifest) == 0 {
		return nil, truncated
	}
	if err := upload(ctx, e.artifactGrant.URL, archive); err != nil {

		m.log.Error("artifact upload failed", "provider_run_id", id,
			"object_key", e.artifactGrant.ObjectKey, "err", err)
		return nil, truncated
	}
	m.log.Info("artifacts collected", "provider_run_id", id, "files", len(manifest))
	return manifest, truncated
}

func filterArchive(raw []byte, limits ResourceLimits) ([]Artifact, []byte, bool, error) {
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
	seen := map[string]struct{}{}
	var used int64
	dropped := false

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {

			dropped = true
			break
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if len(manifest) >= artifactMaxEntries {

			dropped = true
			break
		}
		name := artifactName(header.Name)
		if name == "" {
			dropped = true
			continue
		}
		key := strings.ToLower(name)
		if _, duplicate := seen[key]; duplicate {
			dropped = true
			continue
		}
		if header.Size > perFile || used+header.Size > total {
			dropped = true

			continue
		}

		seen[key] = struct{}{}
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
			return nil, nil, dropped, err
		}
		if _, err := writer.Write(body); err != nil {
			return nil, nil, dropped, err
		}
		used += int64(len(body))
		manifest = append(manifest, Artifact{
			FileName:    name,
			SizeBytes:   int64(len(body)),
			ContentHash: hex.EncodeToString(sum[:]),
		})
	}
	if err := writer.Close(); err != nil {
		return nil, nil, dropped, err
	}
	if dropped {
		for i := range manifest {
			manifest[i].Truncated = true
		}
	}
	return manifest, out.Bytes(), dropped, nil
}

// artifactName rejects any name that would traverse outside the collection,
// contain control characters, or collide with a Windows-reserved device name
// (con, prn, aux, nul, com1-9, lpt1-9) once case and trailing dots are folded.
func artifactName(raw string) string {
	name := strings.TrimPrefix(strings.ReplaceAll(raw, "\\", "/"), "artifacts/")
	name = strings.TrimPrefix(name, "./")
	switch {
	case name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "-"):
		return ""
	case name != path.Clean(name) || strings.HasPrefix(path.Clean(name), ".."):
		return ""
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, " .") != part || strings.ContainsAny(part, `<>:"|?*`) {
			return ""
		}
		for _, r := range part {
			if r < 0x20 {
				return ""
			}
		}
		base := strings.ToLower(strings.SplitN(part, ".", 2)[0])
		if base == "con" || base == "prn" || base == "aux" || base == "nul" ||
			(len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9') {
			return ""
		}
	}
	return name
}

func upload(ctx context.Context, url string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return errors.New("grant URL is not usable")
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := GrantHTTPClient.Do(req)
	if err != nil {
		return errors.New("object storage could not be reached")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if !successfulUploadStatus(resp.StatusCode) {
		return fmt.Errorf("object storage answered %d", resp.StatusCode)
	}
	return nil
}

func successfulUploadStatus(code int) bool { return code >= 200 && code < 300 }

// GrantHTTPClient reports a redirect response as-is instead of following it,
// since a pre-signed grant URL names exactly one object.
var GrantHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
