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
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/stub":
			// The enrichment check's detail view. Enriched, so these tests stay
			// about what they were about.
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

// The launcher must refuse to start when the harness cannot import its own
// runtime, and it must derive the fix from the Dockerfile rather than repeat it.
//
// 2026-08-30: clean test mode had never executed a workload and could not have.
// run.mjs is COPY'd into the runtime image, where the Dockerfile npm-installs
// the Agent SDK beside it; clean mode runs that same script from the repo, which
// carries no node_modules there. Every Run reached `running` and then died with
// `Cannot find package '@anthropic-ai/claude-agent-sdk'` — a fact the launcher
// could have stated before printing its first line, and 02:PORT-005 asks exactly
// that of every preflight failure. The four checks it did have were all about
// getting the processes up.
//
// Pinned here rather than by running the launcher because the paths it checks
// are derived from its own location: there is no repo root to point a test at.
// What this can hold is that the three files still agree, which is the drift
// that would silently un-cover the check.
// 04 丙-102, and the reason it is a test rather than a comment: every one of
// these settings was missing on 2026-08-30's measured launch, and the mode came
// up looking healthy. A launcher that asks for a value it can derive is how the
// value ends up unset, and a launcher that stays silent about the ones it cannot
// derive is how the operator meets them one 503 at a time.
//
// Scoped to the functions that do the work, for the reason the test below is:
// the words appear in this file's own comments, so a whole-file search would
// pass with the code deleted.
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

	// Category 1: derived here because this script owns both ends. Each of these
	// was being asked for, and each was missing on the measured launch.
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
	// It fills gaps rather than overriding: an operator who set one keeps it.
	applied := body("function applyOwnedSettings() {")
	if !strings.Contains(applied, "if (!process.env[name])") {
		t.Error("applyOwnedSettings() no longer leaves an operator's own value alone")
	}

	// Category 3 is reported, never filled in. A default for the retention would
	// be this script deciding a promise on the owner's behalf
	// (GOV-RETENTION-001), so it must appear in the report and NOT in the
	// derived settings.
	if strings.Contains(owned, "DOWNLOAD_ARTIFACT_RETENTION") {
		t.Error("ownedSettings() invents a DOWNLOAD_ARTIFACT_RETENTION: that value is a retention promise quoted to " +
			"users in the consent form, and GOV-RETENTION-001 leaves it unset on purpose")
	}
	table := launcher[strings.Index(launcher, "const CAPABILITIES = ["):]
	if end := strings.Index(table, "\n];\n"); end > 0 {
		table = table[:end]
	}
	for _, name := range []string{
		"DOWNLOAD_ARTIFACT_RETENTION", "LLM_SERVICE_URL",
		"SKILLHUB_MODEL_GATEWAY_URL", "SKILLHUB_MODEL_GATEWAY_KEY", "OPERATOR_USER_IDS",
	} {
		if !strings.Contains(table, name) {
			t.Errorf("the capability report no longer names %s, so a launch missing it says nothing", name)
		}
	}

	// The one refusal. Everything else is a smaller deployment that says so;
	// this one is incoherent, and it kills every run a minute after it starts.
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

	// The assertions below are scoped to preflight()'s own body, not to the file.
	// The first version of this test searched the whole file and stayed green
	// when the entire check was deleted, because the package name still appeared
	// in a helper's comment further down. A test that passes with the code
	// removed is the defect it was written to prevent.
	start := strings.Index(launcher, "async function preflight() {")
	if start < 0 {
		t.Fatal("tools/cleanmode/start.mjs no longer defines preflight(); this test cannot tell what it checks")
	}
	end := strings.Index(launcher[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of preflight() in tools/cleanmode/start.mjs")
	}
	preflight := launcher[start : start+end]

	// The package run.mjs actually imports. If it is renamed and the launcher is
	// not, the check goes on passing while covering nothing.
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

	// ADR-023 決策 1: the Dockerfile's ARG is where that version is written down.
	// A literal here would let clean mode rehearse a different runtime than the
	// image, which is the one thing this mode exists to avoid.
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

// 04 丙-108's unlanded half. A deployment that imports without enriching answers
// every upload with 201 and leaves each search document `pending`, so the loop
// finishes with `imported=50 failed=0` over a catalogue that cannot answer a
// single intent query — and nothing here can repair it afterwards, because
// cmd/reindex reads the object store through OBJSTORE_* and clean mode's store
// lives inside the API process.
//
// Two assertions, and the second is the point: it must refuse, and it must
// refuse after ONE upload. Discovering this at the end costs the whole seed —
// ten minutes and a model bill — for an answer available after the first
// package.
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
			// The detail view of the package just uploaded, in the state an
			// import-without-enrichment leaves it: no model summary, no task
			// examples, no embedding.
			//
			// Deliberately NOT partial_index on a search. The first version of
			// this stub returned that, and it was a value the real platform
			// cannot return in this situation -- partial_index is set from the
			// hybrid leg, and a deployment with no embedding call has no hybrid
			// leg, so it reads false forever exactly here. The check passed its
			// test and could never have fired in production (04 丙-111 again,
			// one commit later).
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
	// 02:PORT-005: name what is missing and how to get it.
	for _, want := range []string{"LLM_SERVICE_URL", "--allow-unindexed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never mentions %q, so it does not say how to recover: %v", want, err)
		}
	}
	if got := atomic.LoadInt32(&uploads); got != 1 {
		t.Errorf("uploads = %d; want 1 — the verdict is available after the first package, and every one after it is spent for nothing", got)
	}

	// The same deployment with the escape hatch typed: a keyword-only catalogue
	// is a shape the launcher's capability table already supports.
	atomic.StoreInt32(&uploads, 0)
	out.Reset()
	if err := seedClean(root, []string{"--allow-unindexed"}, &out); err != nil {
		t.Fatalf("--allow-unindexed still refused: %v", err)
	}
	if got, want := atomic.LoadInt32(&uploads), int32(seedExpectedUploads()); got != want {
		t.Errorf("uploads = %d; want %d", got, want)
	}
}
