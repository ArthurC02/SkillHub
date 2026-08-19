package sandbox

// The artifact collection is the one place this service opens bytes a workload
// wrote, so the limits and the name handling are checked here rather than only
// end to end.

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tarOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{
			Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFilterArchiveEnforcesTheRunCeilings(t *testing.T) {
	limits := ResourceLimits{ArtifactFileBytes: 16, ArtifactTotalBytes: 24}
	raw := tarOf(t, map[string][]byte{
		"artifacts/small.txt": []byte("ok"),
		"artifacts/big.txt":   bytes.Repeat([]byte("x"), 32), // over the per-file ceiling
	})

	manifest, archive, err := filterArchive(raw, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].FileName != "small.txt" {
		t.Fatalf("manifest = %#v, want only small.txt", manifest)
	}
	// A collection that lost something must say so, or the UI shows a partial
	// result as complete.
	if !manifest[0].Truncated {
		t.Error("a dropped file did not mark the collection as truncated")
	}
	if manifest[0].SizeBytes != 2 || manifest[0].ContentHash == "" {
		t.Errorf("manifest entry = %#v, want the real size and a hash", manifest[0])
	}
	// The uploaded archive must carry exactly the manifest: an entry whose bytes
	// were never uploaded would be a manifest that lies.
	names := map[string]bool{}
	r := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := r.Next()
		if err != nil {
			break
		}
		names[h.Name] = true
	}
	if len(names) != 1 || !names["small.txt"] {
		t.Errorf("uploaded archive holds %v, want only small.txt", names)
	}
}

func TestFilterArchiveRefusesNamesThatEscapeTheCollection(t *testing.T) {
	raw := tarOf(t, map[string][]byte{
		"artifacts/../../etc/passwd": []byte("no"),
		"/absolute":                  []byte("no"),
		"artifacts/keep.txt":         []byte("yes"),
	})
	manifest, _, err := filterArchive(raw, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].FileName != "keep.txt" {
		t.Fatalf("manifest = %#v, want only keep.txt", manifest)
	}
}

// The handshake is what makes collection possible at all: /out is a tmpfs, so a
// workload that is already gone has nothing left to take. This checks the order
// - collect, then release - because releasing first would race the workload's
// exit against the read.
func TestCollectionHappensBeforeTheWorkloadIsReleased(t *testing.T) {
	var uploaded []byte
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		uploaded = body
		w.WriteHeader(http.StatusOK)
	}))
	defer store.Close()

	drv := &collectDriver{
		artifacts: tarOf(t, map[string][]byte{"artifacts/out.txt": []byte("result")}),
	}
	m := NewManager(drv, Config{Provider: "test", Slots: 1}, slog.New(slog.DiscardHandler))
	e := &entry{
		limits:        DefaultLimits,
		artifactGrant: &ObjectGrant{Purpose: "artifact_upload", Access: "write", ObjectKey: "k", URL: store.URL + "/k"},
	}
	m.runs["run-1"] = e

	if !m.collect("run-1", "") {
		t.Fatal("collect reported unfinished for a workload that had announced it was done")
	}
	if len(e.artifacts) != 1 || e.artifacts[0].FileName != "out.txt" {
		t.Fatalf("manifest = %#v, want the one collected file", e.artifacts)
	}
	if !bytes.Contains(uploaded, []byte("result")) {
		t.Error("the archive that reached object storage does not carry the file's bytes")
	}
	if drv.releasedAt <= drv.readAt {
		t.Error("the workload was released before its output was read")
	}
}

// collectDriver is the collection half of the Driver interface; the rest panics
// because this test drives no lifecycle.
type collectDriver struct {
	Driver
	artifacts  []byte
	seq        int
	readAt     int
	releasedAt int
}

func (d *collectDriver) WorkloadDone(context.Context, string) (bool, error) { return true, nil }

func (d *collectDriver) ReadArtifacts(context.Context, string) ([]byte, error) {
	d.seq++
	d.readAt = d.seq
	return d.artifacts, nil
}

func (d *collectDriver) ReadTrace(context.Context, string) ([]byte, error) { return nil, nil }

func (d *collectDriver) ReleaseWorkload(context.Context, string) error {
	d.seq++
	d.releasedAt = d.seq
	return nil
}

// An empty allow list must not be servable by a node that has an egress route
// either way round: the ordering is what SBX-007 and the contract both state.
func TestAcceptRefusesAnAllowListANodeCannotRoute(t *testing.T) {
	cfg := Config{
		Runtimes:     []RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"1"}}},
		MaxResources: DefaultLimits,
		EgressModes:  []string{"none"},
	}
	req := RunRequest{
		Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
		ResourceLimits: DefaultLimits,
		Egress:         EgressPolicy{Mode: "default_deny", Allow: []EgressAllowEntry{{Purpose: "model_gateway", URL: "http://gw:4000"}}},
	}
	if re := cfg.accept(req); re == nil || re.Class != ClassCapabilityMismatch {
		t.Fatalf("a node with no egress route accepted a destination: %v", re)
	}
	// The same node still carries a run that is allowed to reach nothing.
	req.Egress.Allow = nil
	if re := cfg.accept(req); re != nil {
		t.Fatalf("a node with no egress route refused a run that needs none: %v", re)
	}
}

func TestAcceptChecksEveryDeclaredResourceCeiling(t *testing.T) {
	checks := map[string]func(*RunRequest, *Config){
		"vcpu":                func(r *RunRequest, _ *Config) { r.ResourceLimits.VCPU++ },
		"memory":              func(r *RunRequest, _ *Config) { r.ResourceLimits.MemoryBytes++ },
		"disk":                func(r *RunRequest, _ *Config) { r.ResourceLimits.DiskBytes++ },
		"processes":           func(r *RunRequest, _ *Config) { r.ResourceLimits.MaxPIDs++ },
		"open files":          func(r *RunRequest, _ *Config) { r.ResourceLimits.MaxOpenFiles++ },
		"soft wall clock":     func(r *RunRequest, _ *Config) { r.ResourceLimits.WallClockSoftSeconds++ },
		"hard wall clock":     func(r *RunRequest, _ *Config) { r.ResourceLimits.WallClockHardSeconds++ },
		"artifact total":      func(r *RunRequest, _ *Config) { r.ResourceLimits.ArtifactTotalBytes++ },
		"artifact file":       func(r *RunRequest, _ *Config) { r.ResourceLimits.ArtifactFileBytes++ },
		"input tokens":        func(r *RunRequest, _ *Config) { r.ResourceLimits.TokenBudget.MaxInputTokens++ },
		"output tokens":       func(r *RunRequest, _ *Config) { r.ResourceLimits.TokenBudget.MaxOutputTokens++ },
		"missing token limit": func(_ *RunRequest, c *Config) { c.MaxResources.TokenBudget = nil },
	}
	for name, exceed := range checks {
		t.Run(name, func(t *testing.T) {
			max := DefaultLimits
			maxTokens := *DefaultLimits.TokenBudget
			max.TokenBudget = &maxTokens
			requested := DefaultLimits
			requestedTokens := *DefaultLimits.TokenBudget
			requested.TokenBudget = &requestedTokens
			cfg := Config{
				Runtimes:     []RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"1"}}},
				MaxResources: max, EgressModes: []string{"none"},
			}
			req := RunRequest{
				Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
				ResourceLimits: requested, Egress: EgressPolicy{Mode: "none"},
			}
			exceed(&req, &cfg)
			if got := cfg.accept(req); got == nil || got.Class != ClassCapabilityMismatch {
				t.Fatalf("provider accepted a run it cannot enforce: %v", got)
			}
		})
	}
	missing := map[string]func(*ResourceLimits){
		"vcpu":            func(l *ResourceLimits) { l.VCPU = 0 },
		"memory":          func(l *ResourceLimits) { l.MemoryBytes = 0 },
		"disk":            func(l *ResourceLimits) { l.DiskBytes = 0 },
		"processes":       func(l *ResourceLimits) { l.MaxPIDs = 0 },
		"open files":      func(l *ResourceLimits) { l.MaxOpenFiles = 0 },
		"soft wall clock": func(l *ResourceLimits) { l.WallClockSoftSeconds = 0 },
		"hard wall clock": func(l *ResourceLimits) { l.WallClockHardSeconds = 0 },
		"artifact total":  func(l *ResourceLimits) { l.ArtifactTotalBytes = 0 },
		"artifact file":   func(l *ResourceLimits) { l.ArtifactFileBytes = 0 },
	}
	for name, omit := range missing {
		t.Run("provider omits "+name, func(t *testing.T) {
			max := DefaultLimits
			omit(&max)
			cfg := Config{
				Runtimes:     []RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"1"}}},
				MaxResources: max, EgressModes: []string{"none"},
			}
			req := RunRequest{
				Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
				ResourceLimits: DefaultLimits, Egress: EgressPolicy{Mode: "none"},
			}
			if got := cfg.accept(req); got == nil || got.Class != ClassCapabilityMismatch {
				t.Fatalf("provider omitted %s but accepted a bounded run: %v", name, got)
			}
		})
	}
}
