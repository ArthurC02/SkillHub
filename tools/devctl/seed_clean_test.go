package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

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

func TestCollectMarkdownSkillsMissingFileFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(gen009SkillsRelDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

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
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/stub":

			_, _ = fmt.Fprint(w, `{"skill_id":"stub","enrichment":{"status":"enriched"}}`)
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

func TestPartitionSeedEntriesRejectsAStaleExclusion(t *testing.T) {
	_, _, err := partitionSeedEntries([]seedEntry{{name: "x", provenance: "tools/goldenset/corpus/moved.md"}})
	if err == nil {
		t.Fatal("expected an error for an exclusion that matched nothing, got nil")
	}
	if !strings.Contains(err.Error(), "matched no collected file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

func TestTheLauncherSuppliesWhatItOwnsAndNamesWhatItCannot(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "tools", "cleanmode", "start.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := string(raw)

	body := func(header string) string {
		start := strings.Index(launcher, header)
		if start < 0 {
			t.Fatalf("tools/cleanmode/start.mjs no longer defines %q; this test cannot tell what it does", header)
		}
		end := strings.Index(launcher[start:], "\n}\n")
		if end < 0 {
			t.Fatalf("could not find the end of %q in tools/cleanmode/start.mjs", header)
		}
		return launcher[start : start+end]
	}

	owned := body("function ownedSettings() {")
	for name, cost := range map[string]string{
		"SKILLHUB_TRACE_INGEST_SECRET": "a failed run says only `workload exited with code 1`",
		"SKILLHUB_TRACE_INGEST_URL":    "the sandbox posts no trace events at all",
		"PACKAGING_PROFILES_DIR":       "the platform's default resolves from the wrong cwd and packaging reports itself unconfigured",
	} {
		if !strings.Contains(owned, name) {
			t.Errorf("ownedSettings() no longer supplies %s; without it %s", name, cost)
		}
	}

	applied := body("function applyOwnedSettings() {")
	if !strings.Contains(applied, "if (!deployment(name))") {
		t.Error("applyOwnedSettings() no longer leaves an operator's own value alone")
	}

	if strings.Contains(owned, "DOWNLOAD_ARTIFACT_RETENTION") {
		t.Error("ownedSettings() invents a DOWNLOAD_ARTIFACT_RETENTION: that value is a retention promise quoted to " +
			"users in the consent form, and GOV-RETENTION-001 leaves it unset on purpose")
	}

	if strings.Contains(launcher, "const CAPABILITIES = [") {
		t.Error("the launcher holds a capability list again. 05 R-36's hard condition is that it reads the " +
			"platform's answer; two lists of the same preconditions is the drift this repo keeps finding")
	}
	if !strings.Contains(launcher, "/readyz") {
		t.Error("the launcher no longer asks the platform what this deployment can do")
	}

	for _, state := range []string{"unmeasured", "unavailable", "broken", "ready"} {
		if !strings.Contains(launcher, state) {
			t.Errorf("the launcher does not render the %q state, so it collapses back into the others", state)
		}
	}

	table, err := os.ReadFile(filepath.Join(root, "apps", "platform", "cmd", "api", "capabilities.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"DOWNLOAD_ARTIFACT_RETENTION", "LLM_SERVICE_URL",
		"SKILLHUB_MODEL_GATEWAY_URL", "SKILLHUB_MODEL_GATEWAY_KEY", "OPERATOR_USER_IDS",
	} {
		if !strings.Contains(string(table), name) {
			t.Errorf("the capability table no longer names %s, so a launch missing it says nothing", name)
		}
	}

	preflight := body("async function preflight() {")
	if !strings.Contains(preflight, "SKILLHUB_RUN_MODEL") ||
		!strings.Contains(preflight, "SKILLHUB_MODEL_GATEWAY_URL") {
		t.Error("preflight() no longer refuses a gateway with no SKILLHUB_RUN_MODEL: the Agent SDK then asks for its " +
			"own default model, which the gateway does not serve, and every run dies on `400 Invalid model name`")
	}
}

func TestTheLauncherRefusesToStartWithoutTheHarnessRuntime(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	read := func(parts ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	launcher := read("tools", "cleanmode", "start.mjs")
	harness := read("infra", "images", "runtime-agent-sdk", "run.mjs")

	start := strings.Index(launcher, "async function preflight() {")
	if start < 0 {
		t.Fatal("tools/cleanmode/start.mjs no longer defines preflight(); this test cannot tell what it checks")
	}
	end := strings.Index(launcher[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of preflight() in tools/cleanmode/start.mjs")
	}
	preflight := launcher[start : start+end]

	pkg := regexp.MustCompile(`import\("(@[^"]+/[^"]+)"\)`).FindStringSubmatch(harness)
	if pkg == nil {
		t.Fatal("run.mjs no longer dynamically imports a scoped package; this test can no longer tell what the launcher must check for")
	}
	if !strings.Contains(preflight, pkg[1]) {
		t.Fatalf("run.mjs imports %q but preflight() never checks for it: clean mode would accept a Run and fail it after dispatch (04 丙-100)", pkg[1])
	}
	if !strings.Contains(preflight, "node_modules") {
		t.Fatal("preflight() no longer checks for an installed dependency tree beside run.mjs")
	}

	if !strings.Contains(launcher, "CLAUDE_AGENT_SDK_VERSION") {
		t.Fatal("tools/cleanmode/start.mjs no longer reads CLAUDE_AGENT_SDK_VERSION from the Dockerfile; a second copy of that version is how clean mode stops rehearsing the image (ADR-023 決策 1)")
	}
	if !strings.Contains(preflight, "agentSdkVersion(") {
		t.Fatal("preflight() no longer builds its hint from the Dockerfile's pinned version, so the fix it prints can name a runtime the image does not have")
	}
	version := regexp.MustCompile(`(?m)^ARG\s+CLAUDE_AGENT_SDK_VERSION\s*=\s*"?([^"\s]+)"?\s*$`).
		FindStringSubmatch(read("infra", "images", "runtime-agent-sdk", "Dockerfile"))
	if version == nil {
		t.Fatal("the Dockerfile no longer declares ARG CLAUDE_AGENT_SDK_VERSION")
	}
	if strings.Contains(launcher, version[1]) {
		t.Fatalf("tools/cleanmode/start.mjs hard-codes the SDK version %q instead of reading it from the Dockerfile", version[1])
	}
}

func TestSeedCleanStopsAtTheFirstUnindexedPackage(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/search":
			_, _ = fmt.Fprintf(w, `{"query":%q,"results":[],"total":1}`, r.URL.Query().Get("q"))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/skills/"):

			_, _ = fmt.Fprint(w, `{"skill_id":"stub","enrichment":{"status":"pending"}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("SKILLHUB_API", server.URL)

	var out strings.Builder
	err = seedClean(root, nil, &out)
	if err == nil {
		t.Fatal("seed-clean accepted a deployment that imported without enriching; the catalogue it just built cannot answer an intent query and cannot be repaired")
	}

	if !strings.Contains(err.Error(), "LLM_SERVICE_URL") {
		t.Errorf("the refusal never mentions LLM_SERVICE_URL, so it does not say how to recover: %v", err)
	}
	for _, stale := range []string{"--allow-unindexed", "keyword search will find it", "not fixable after the fact"} {
		if strings.Contains(err.Error(), stale) {
			t.Errorf("the refusal still says %q, which stopped being true when pending documents left every catalog query: %v", stale, err)
		}
	}
	if got := atomic.LoadInt32(&uploads); got != 1 {
		t.Errorf("uploads = %d; want 1 — the verdict is available after the first package, and every one after it is spent for nothing", got)
	}

	atomic.StoreInt32(&uploads, 0)
	out.Reset()
	err = seedClean(root, []string{"--allow-unindexed"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("--allow-unindexed was accepted; it can only seed a catalog nobody can see: %v", err)
	}
	if got := atomic.LoadInt32(&uploads); got != 0 {
		t.Errorf("uploads = %d; want 0 — a refused argument must be refused before anything is sent", got)
	}
}
