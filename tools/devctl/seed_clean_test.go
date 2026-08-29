package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// The two real batches PORT-007 allows (docs/plans/03-work-items.md §20):
// this test runs against the actual repo tree, not a fixture, because the
// point of the check is that these specific files exist and are traceable.
func TestCollectSeedEntriesReadsBothRealBatches(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := collectSeedEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != gen009ExpectedCount+goldensetExpectedCount {
		t.Fatalf("got %d entries; want %d (%d gen009 + %d goldenset)",
			len(entries), gen009ExpectedCount+goldensetExpectedCount, gen009ExpectedCount, goldensetExpectedCount)
	}

	gen009, goldenset := 0, 0
	for _, e := range entries {
		if strings.HasPrefix(e.provenance, filepath.ToSlash(gen009SkillsRelDir)) {
			gen009++
		}
		if strings.HasPrefix(e.provenance, filepath.ToSlash(goldensetCorpusRelDir)) {
			goldenset++
		}
		// The requirement (02:PORT-007) is that every provenance string
		// actually resolves to a committed file — check the byte, not the
		// shape of the string.
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(e.provenance)))
		if err != nil {
			t.Fatalf("entry %q: provenance %q does not resolve: %v", e.name, e.provenance, err)
		}
		if info.IsDir() {
			t.Fatalf("entry %q: provenance %q is a directory, not a file", e.name, e.provenance)
		}
		if len(e.skillMD) == 0 {
			t.Fatalf("entry %q: skillMD is empty", e.name)
		}
	}
	if gen009 != gen009ExpectedCount {
		t.Errorf("gen009 entries = %d; want %d", gen009, gen009ExpectedCount)
	}
	if goldenset != goldensetExpectedCount {
		t.Errorf("goldenset entries = %d; want %d", goldenset, goldensetExpectedCount)
	}
}

// A missing source file must fail the whole command, never silently seed
// fewer entries than the recorded batch size (02:PORT-007: "指不回去的即不得使用").
func TestCollectMarkdownSkillsMissingFileFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(gen009SkillsRelDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// One short of gen009ExpectedCount.
	for i := 0; i < gen009ExpectedCount-1; i++ {
		writeSkillMD(t, filepath.Join(dir, fmt.Sprintf("skill-%d.md", i)), fmt.Sprintf("skill-%d", i))
	}

	_, err := collectMarkdownSkills(root, gen009SkillsRelDir, gen009ExpectedCount)
	if err == nil {
		t.Fatal("expected an error for a short batch, got nil")
	}
	if !strings.Contains(err.Error(), "want 20") {
		t.Fatalf("error does not name the expected count: %v", err)
	}
}

// A file that is not a SKILL.md (no frontmatter) must also fail loudly.
func TestCollectMarkdownSkillsRejectsNonSkillFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(gen009SkillsRelDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < gen009ExpectedCount-1; i++ {
		writeSkillMD(t, filepath.Join(dir, fmt.Sprintf("skill-%d.md", i)), fmt.Sprintf("skill-%d", i))
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-skill.md"), []byte("just some prose\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := collectMarkdownSkills(root, gen009SkillsRelDir, gen009ExpectedCount)
	if err == nil {
		t.Fatal("expected an error for a non-SKILL.md file, got nil")
	}
	if !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeSkillMD(t *testing.T, path, name string) {
	t.Helper()
	content := fmt.Sprintf("---\nname: %s\ndescription: fixture skill for devctl seed-clean tests.\n---\n\nbody\n", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --dry-run is the only part of PORT-007's acceptance criteria a platform-less
// checkout can verify — it must send zero requests.
func TestSeedCleanDryRunSendsNoRequests(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		t.Errorf("unexpected request during --dry-run: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	if err := seedClean(root, []string{"--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("--dry-run sent %d request(s); want 0", got)
	}
	want := fmt.Sprintf("%d skill(s)", gen009ExpectedCount+goldensetExpectedCount)
	if !strings.Contains(out.String(), want) {
		t.Fatalf("dry-run output does not report the count: %q", out.String())
	}
	// Every printed line must carry a provenance= marker so a reader can trace
	// each planned upload back to its source file without running anything.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1+gen009ExpectedCount+goldensetExpectedCount {
		t.Fatalf("got %d output lines; want a header plus one per entry", len(lines))
	}
	for _, l := range lines[1:] {
		if !strings.Contains(l, "source=") {
			t.Fatalf("line missing source= provenance marker: %q", l)
		}
	}
}

func TestSeedCleanUploadsEveryEntry(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	var logins, uploads int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/dev/login":
			atomic.AddInt32(&logins, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/skills/import/upload":
			atomic.AddInt32(&uploads, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"skill_id":"stub"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	if err := seedClean(root, nil, &out); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&logins); got != 1 {
		t.Fatalf("dev login called %d time(s); want 1", got)
	}
	want := int32(gen009ExpectedCount + goldensetExpectedCount)
	if got := atomic.LoadInt32(&uploads); got != want {
		t.Fatalf("uploads = %d; want %d", got, want)
	}
}

func TestSeedCleanFailsOnUploadError(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/dev/login":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/skills/import/upload":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":[{"code":"bad"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	if err := seedClean(root, nil, &out); err == nil {
		t.Fatal("expected an error when every upload is rejected, got nil")
	}
}
