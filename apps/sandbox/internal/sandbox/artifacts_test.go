package sandbox

// The artifact collection is the one place this service opens bytes a workload
// wrote, so the limits and the name handling are checked here rather than only
// end to end.

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	// One subtest per half of the condition, and each fixture is refused by its
	// own half alone: a per-file case that stays under the run total, and a run
	// total case whose files are each within the per-file ceiling. A fixture
	// that trips both halves leaves one of them untested.
	t.Run("per-file ceiling", func(t *testing.T) {
		limits := ResourceLimits{ArtifactFileBytes: 16, ArtifactTotalBytes: 1 << 20}
		raw := tarOf(t, map[string][]byte{
			"artifacts/small.txt": []byte("ok"),
			"artifacts/big.txt":   bytes.Repeat([]byte("x"), 32), // over the per-file ceiling, far under the run total
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
	})

	t.Run("run total ceiling", func(t *testing.T) {
		// The real shape of this attack is many small files, not one big one:
		// both of these pass the per-file ceiling and only their sum breaks the
		// run's. Symmetric on purpose - tar entry order is map order here, and
		// whichever lands second is the one that must be dropped.
		limits := ResourceLimits{ArtifactFileBytes: 16, ArtifactTotalBytes: 12}
		raw := tarOf(t, map[string][]byte{
			"artifacts/a.txt": bytes.Repeat([]byte("a"), 8),
			"artifacts/b.txt": bytes.Repeat([]byte("b"), 8),
		})

		manifest, _, err := filterArchive(raw, limits)
		if err != nil {
			t.Fatal(err)
		}
		if len(manifest) != 1 {
			t.Fatalf("manifest = %#v, want one file: 16 bytes do not fit in a 12 byte run ceiling", manifest)
		}
		if manifest[0].SizeBytes != 8 || !manifest[0].Truncated {
			t.Errorf("manifest entry = %#v, want 8 bytes and the truncated mark", manifest[0])
		}
	})
}

// Bytes are not the only dimension a workload controls. Empty files cost
// nothing against either byte ceiling and still take a manifest entry each, and
// a manifest too large for the platform to read back loses the whole run
// result - so the entry count is bounded too.
func TestFilterArchiveBoundsTheNumberOfManifestEntries(t *testing.T) {
	files := map[string][]byte{}
	for i := range artifactMaxEntries + 5 {
		files[fmt.Sprintf("artifacts/f%04d.txt", i)] = nil
	}
	manifest, _, err := filterArchive(tarOf(t, files), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != artifactMaxEntries {
		t.Fatalf("manifest holds %d entries, want the %d ceiling", len(manifest), artifactMaxEntries)
	}
	if !manifest[0].Truncated {
		t.Error("entries were dropped but the collection is not marked truncated")
	}
}

func TestFilterArchiveRefusesNamesThatEscapeTheCollection(t *testing.T) {
	raw := tarOf(t, map[string][]byte{
		"artifacts/../../etc/passwd": []byte("no"),
		"/absolute":                  []byte("no"),
		// A name a downstream CLI would read as a flag rather than a file.
		"artifacts/-rf": []byte("no"),
		// Not path.Clean's own answer: a name that means one thing to the
		// filter and another to whatever opens the archive later.
		"artifacts/a//b.txt": []byte("no"),
		// Backslashes are normalised to "/" before any of the checks run;
		// without that, this escapes the collection as one long file name.
		`artifacts\..\..\etc\passwd`: []byte("no"),
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

func (d *collectDriver) ReadTrace(context.Context, string, int64) ([]byte, bool, error) {
	return nil, false, nil
}

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
			// The class alone cannot tell the two guards apart: the requested
			// DefaultLimits also exceed a zeroed ceiling, so the per-field
			// comparison would answer with the same class and this table would
			// pass with the omitted-ceiling guard deleted.
			got := cfg.accept(req)
			if got == nil || got.Class != ClassCapabilityMismatch {
				t.Fatalf("provider omitted %s but accepted a bounded run: %v", name, got)
			}
			if !strings.Contains(got.Message, "must declare every resource ceiling") {
				t.Fatalf("provider omitted %s and was refused for the wrong reason: %q", name, got.Message)
			}
		})
	}
}

// TestUploadRefusesToFollowARedirect covers the third caller of
// GrantHTTPClient — the artifact upload, the one direction that carries the
// run's own output. A PUT that followed a 302 would replay the body at
// whatever address the storage endpoint named.
func TestUploadRefusesToFollowARedirect(t *testing.T) {
	var elsewhereHits int
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhereHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer srv.Close()

	err := upload(context.Background(), srv.URL+"/signed", []byte("artifact bytes"))
	if err == nil {
		t.Fatal("a redirected upload grant was followed")
	}
	if !strings.Contains(err.Error(), "302") {
		t.Errorf("the refusal should report the redirect status it saw; got %v", err)
	}
	if elsewhereHits != 0 {
		t.Errorf("the redirect target received %d request(s); a pre-signed PUT must not be replayed elsewhere", elsewhereHits)
	}
}
