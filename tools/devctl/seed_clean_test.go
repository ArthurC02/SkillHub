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
	want := fmt.Sprintf("%d skill(s)", seedExpectedUploads())
	if !strings.Contains(out.String(), want) {
		t.Fatalf("dry-run output does not report the count: %q", out.String())
	}
	// Every planned upload must carry a provenance= marker so a reader can trace
	// it back to its source file without running anything; the exclusions follow,
	// two lines each (the path, then why it is not going).
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1+seedExpectedUploads()+2*len(seedExclusions) {
		t.Fatalf("got %d output lines; want a header, one per entry and two per exclusion", len(lines))
	}
	for _, l := range lines[1 : 1+seedExpectedUploads()] {
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
	server := httptest.NewServer(http.HandlerFunc(seedStubHandler(t, &logins, &uploads, 1)))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	if err := seedClean(root, nil, &out); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&logins); got != 1 {
		t.Fatalf("dev login called %d time(s); want 1", got)
	}
	want := int32(seedExpectedUploads())
	if got := atomic.LoadInt32(&uploads); got != want {
		t.Fatalf("uploads = %d; want %d", got, want)
	}
}

// seedStubHandler stands in for the platform: dev login, uploads, and the
// public catalog search answering with searchTotal.
func seedStubHandler(t *testing.T, logins, uploads *int32, searchTotal int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/dev/login":
			atomic.AddInt32(logins, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/skills/import/upload":
			atomic.AddInt32(uploads, 1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"skill_id":"stub"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/search":
			if r.URL.Query().Get("q") == "" {
				t.Errorf("catalog check sent no q")
			}
			_, _ = fmt.Fprintf(w, `{"query":%q,"results":[],"total":%d}`, r.URL.Query().Get("q"), searchTotal)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// 04 丙-84 ①: fifty packages landed and the demo's own screen showed nothing.
// A seed that cannot be found is a failed seed, not a quiet success.
func TestSeedCleanFailsWhenTheCatalogSearchFindsNothing(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	var logins, uploads int32
	server := httptest.NewServer(http.HandlerFunc(seedStubHandler(t, &logins, &uploads, 0)))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	err = seedClean(root, nil, &out)
	if err == nil {
		t.Fatal("expected an error when the catalog search finds nothing, got nil")
	}
	if !strings.Contains(err.Error(), "is_catalog") {
		t.Fatalf("error does not name the flag that decides visibility: %v", err)
	}
	if got := atomic.LoadInt32(&uploads); got != int32(seedExpectedUploads()) {
		t.Fatalf("uploads = %d; want %d — the check must run after a full upload, not instead of one", got, seedExpectedUploads())
	}
}

// 04 丙-84 ②: the one file the platform's spec validator refuses. It must be
// left alone, it must not be uploaded, and its absence must be said out loud.
func TestSeedCleanExcludesTheUnvalidatablePackageAndSaysWhy(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	all, err := collectSeedEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	upload, excluded, err := partitionSeedEntries(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(excluded) != len(seedExclusions) {
		t.Fatalf("excluded %d entries; want %d", len(excluded), len(seedExclusions))
	}
	if len(upload)+len(excluded) != len(all) {
		t.Fatalf("partition lost entries: %d + %d != %d", len(upload), len(excluded), len(all))
	}
	for _, e := range upload {
		if _, ok := seedExclusions[e.provenance]; ok {
			t.Fatalf("%s is excluded but still in the upload set", e.provenance)
		}
	}

	var out strings.Builder
	if err := seedClean(root, []string{"--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	for path, reason := range seedExclusions {
		if !strings.Contains(out.String(), path) {
			t.Fatalf("output never names the excluded file %s: %q", path, out.String())
		}
		if !strings.Contains(out.String(), reason) {
			t.Fatalf("output never gives the reason %s is excluded", path)
		}
	}
}

// An exclusion that matches nothing has stopped excluding anything, and only a
// failure says so.
func TestPartitionSeedEntriesRejectsAStaleExclusion(t *testing.T) {
	_, _, err := partitionSeedEntries([]seedEntry{{name: "x", provenance: "tools/goldenset/corpus/moved.md"}})
	if err == nil {
		t.Fatal("expected an error for an exclusion that matched nothing, got nil")
	}
	if !strings.Contains(err.Error(), "matched no collected file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The flag this whole command depends on is granted by the clean-mode launcher,
// in the one window where anything can write it (the carrier serves a single
// client, ADR-060 決策 2). Nothing in Go can reach that code, so what is pinned
// here is that it is still there and still talking about this account.
func TestTheLauncherGrantsTheSeedImporterACatalogWorkspace(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := os.ReadFile(filepath.Join(root, "tools", "cleanmode", "start.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(launcher)
	for _, needle := range []string{seedDevLoginUser, "is_catalog"} {
		if !strings.Contains(src, needle) {
			t.Fatalf("tools/cleanmode/start.mjs no longer mentions %q — without it seed-clean uploads land where the catalog search cannot see them (04 丙-84 ①)", needle)
		}
	}
	// Defining the grant is not calling it, and calling it late is not calling
	// it: once the API is spawned it holds the carrier's single connection, so
	// the SQL has nowhere left to run.
	grant := strings.Index(src, "await grantCatalogWorkspace(")
	api := strings.Index(src, `start("api"`)
	if grant < 0 {
		t.Fatal("tools/cleanmode/start.mjs never calls grantCatalogWorkspace — the seed importer's workspace stays private (04 丙-84 ①)")
	}
	if api < 0 {
		t.Fatal(`tools/cleanmode/start.mjs no longer starts the API with start("api"; this check can no longer tell whether the grant happens first`)
	}
	if grant > api {
		t.Fatal("tools/cleanmode/start.mjs grants the catalog workspace after starting the API; by then the API holds the carrier's only connection (ADR-060 決策 2) and the statement cannot run")
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
