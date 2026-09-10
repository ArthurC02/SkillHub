package apiserver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

const embedDims = 1536

func requireInterfaceLanguage(t *testing.T, what, s string) {
	t.Helper()
	if s == "" {
		t.Fatalf("%s is empty", what)
	}
	if !strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		t.Fatalf("%s is not in the interface language: %q", what, s)
	}
}

func markCatalog(t *testing.T, pool *pgxpool.Pool, workspaceID string) {
	t.Helper()
	var ws pgtype.UUID
	if err := ws.Scan(workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE workspaces SET is_catalog = true WHERE id = $1", ws,
	); err != nil {
		t.Fatal(err)
	}
}

func seedSkillVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID string) string {
	t.Helper()
	var ws, sk pgtype.UUID
	if err := ws.Scan(workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	ver, err := gen.New(pool).CreateSkillVersion(context.Background(), gen.CreateSkillVersionParams{
		WorkspaceID:      ws,
		SkillID:          sk,
		ContentHash:      "sha256:" + skillID,
		PackageObjectKey: "packages/" + skillID + ".tar",
		Manifest:         []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := ver.ID.Value()
	s, _ := id.(string)
	return s
}

func seedEmbedding(t *testing.T, pool *pgxpool.Pool, skillID string, axis int) {
	t.Helper()
	var sk pgtype.UUID
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	v := unitVector(axis)
	if _, err := pool.Exec(context.Background(),
		"UPDATE search_documents SET embedding = $2 WHERE skill_id = $1", sk, pgvector.NewVector(v),
	); err != nil {
		t.Fatal(err)
	}
}

func unitVector(axis int) []float32 {
	v := make([]float32, embedDims)
	v[axis] = 1
	return v
}

func seedBlendedEmbedding(t *testing.T, pool *pgxpool.Pool, skillID string, axis, other int, cos float64) {
	t.Helper()
	var sk pgtype.UUID
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	v := make([]float32, embedDims)
	v[axis] = float32(cos)
	v[other] = float32(math.Sqrt(1 - cos*cos))
	if _, err := pool.Exec(context.Background(),
		"UPDATE search_documents SET embedding = $2 WHERE skill_id = $1", sk, pgvector.NewVector(v),
	); err != nil {
		t.Fatal(err)
	}
}

func stubLLM(t *testing.T, embedAxis int, reason string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		if embedAxis < 0 {
			http.Error(w, `{"detail":"embedding provider error"}`, http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{
			"embeddings": [][]float32{unitVector(embedAxis)},
			"model":      "text-embedding-3-small",
			"dimensions": embedDims,
		})
	})
	mux.HandleFunc("POST /match-reasons", func(w http.ResponseWriter, r *http.Request) {
		if reason == "" {
			http.Error(w, `{"detail":"model unavailable"}`, http.StatusBadGateway)
			return
		}
		var req struct {
			Candidates []struct {
				SkillID string `json:"skill_id"`
			} `json:"candidates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		out := make([]map[string]string, 0, len(req.Candidates))
		for _, c := range req.Candidates {
			out = append(out, map[string]string{"skill_id": c.SkillID, "reason": reason})
		}
		writeJSON(w, map[string]any{"reasons": out})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestBrowseCatalogScopeOrderFiltersShapeAndNoModelCall(t *testing.T) {
	pool := requireDB(t)
	var modelCalls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modelCalls.Add(1)
		http.Error(w, "model must not be called while browsing", http.StatusInternalServerError)
	}))
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	curator := a.login(t, uniqueWorklistLabel("catalog-browse"))
	markCatalog(t, pool, curator.workspaceID)
	private := a.login(t, uniqueWorklistLabel("catalog-private"))

	curatedID := importPackage(t, pool, a.packages, curator, uniqueWorklistLabel("catalog-curated"), true)
	plainID := importPackage(t, pool, a.packages, curator, uniqueWorklistLabel("catalog-plain"), false)
	otherRuntimeID := importPackage(t, pool, a.packages, curator, uniqueWorklistLabel("catalog-other-runtime"), true)
	noVersionID := seedSkill(t, pool, curator.workspaceID, uniqueWorklistLabel("catalog-no-version"))
	privateID := importPackage(t, pool, a.packages, private, uniqueWorklistLabel("catalog-hidden"), true)
	versionedName := uniqueWorklistLabel("catalog-version-order")
	versionedID := importPackage(t, pool, a.packages, curator, versionedName, true)
	var latestVersion pgtype.UUID
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO skill_versions
			(workspace_id, skill_id, version_number, content_hash, package_object_key,
			 manifest, license_expression, license_source, created_at)
		SELECT workspace_id, skill_id, version_number + 1, content_hash || '-v2',
		       package_object_key, manifest, license_expression, license_source,
		       '2000-01-01'::timestamptz
		FROM skill_versions WHERE skill_id = $1
		RETURNING id`, mustUUID(t, versionedID)).Scan(&latestVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO skill_runtime_compatibility
		(skill_version_id, runtime_image, capability, runtime)
		VALUES ($1, 'ghcr.io/example/runtime@sha256:2222', 'activated', 'native')`, latestVersion); err != nil {
		t.Fatal(err)
	}
	curatedVersion := newestVersion(t, pool, curatedID)
	curate(t, pool, curatedID, curatedVersion)
	for skillID, runtime := range map[string]string{curatedID: "native", otherRuntimeID: "transpiled"} {
		versionID := newestVersion(t, pool, skillID)
		if _, err := pool.Exec(t.Context(), `INSERT INTO skill_runtime_compatibility
			(skill_version_id, runtime_image, capability, runtime)
			VALUES ($1, 'ghcr.io/example/runtime@sha256:1111', 'activated', $2)`, mustUUID(t, versionID), runtime); err != nil {
			t.Fatal(err)
		}
	}

	anon := &client{Client: http.DefaultClient, base: a.URL}
	page := anon.search(t, "/api/skills/catalog?limit=100")
	if page.Total < 5 || len(page.Results) < 5 {
		t.Fatalf("catalog page = total %d rows %d, want at least this test's five rows", page.Total, len(page.Results))
	}
	positions := map[string]int{}
	for i, row := range page.Results {
		positions[row.SkillID] = i
		if row.SkillID == privateID {
			t.Fatal("private workspace row leaked into public catalog")
		}
	}
	curatedPos, ok := positions[curatedID]
	if !ok || curatedPos >= positions[plainID] || curatedPos >= positions[otherRuntimeID] || curatedPos >= positions[noVersionID] {
		t.Fatalf("curated row was not ahead of this test's indexed rows: positions=%v", positions)
	}
	curatedRow := page.Results[curatedPos]
	if curatedRow.Rank != nil || curatedRow.RankNote == "" {
		t.Fatalf("catalog rank shape = %+v", curatedRow)
	}
	versionedPos, ok := positions[versionedID]
	if !ok {
		t.Fatalf("catalog omitted version-order fixture: positions=%v", positions)
	}
	versionedRow := page.Results[versionedPos]
	if versionedRow.Compatibility.Runtime.Value != "native" {
		t.Fatalf("catalog chose an older version by timestamp: %+v", versionedRow.Compatibility)
	}
	limited := anon.search(t, "/api/skills/catalog?limit=2")
	if len(limited.Results) != 2 || !limited.Truncated || limited.Total != page.Total {
		t.Fatalf("limited catalog page = total %d rows %d truncated %v", limited.Total, len(limited.Results), limited.Truncated)
	}
	assertOwnFilter := func(query, want string, reject ...string) {
		t.Helper()
		body := anon.search(t, "/api/skills/catalog?limit=100&"+query)
		found := map[string]bool{}
		for _, row := range body.Results {
			found[row.SkillID] = true
		}
		if !found[want] {
			t.Fatalf("catalog filter %q omitted %s: %+v", query, want, body.Results)
		}
		for _, id := range reject {
			if found[id] {
				t.Fatalf("catalog filter %q retained rejected test row %s", query, id)
			}
		}
	}
	assertOwnFilter("script=no", plainID, curatedID, otherRuntimeID)
	assertOwnFilter("validation=unverified", noVersionID, curatedID, plainID, otherRuntimeID)
	assertOwnFilter("agent=native", versionedID, plainID, otherRuntimeID, noVersionID)
	assertOwnFilter("tier=curated", curatedID, plainID, otherRuntimeID, noVersionID)
	if got := modelCalls.Load(); got != 0 {
		t.Fatalf("browse made %d model calls", got)
	}
}

func importPackage(t *testing.T, pool *pgxpool.Pool, store packageStore, owner *client, name string, withScript bool) string {
	t.Helper()
	ctx := context.Background()
	var wsID, userID pgtype.UUID
	if err := wsID.Scan(owner.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := userID.Scan(owner.userID); err != nil {
		t.Fatal(err)
	}
	ws, err := gen.New(pool).GetWorkspace(ctx, gen.GetWorkspaceParams{ID: wsID, OwnerUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	svc := &ingest.Service{Pool: pool, Store: store, IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
		return catalog.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
			SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
			EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
			Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
			EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
			EnrichmentPromptVersion: p.EnrichmentPromptVersion,
		})
	}}
	res, err := svc.UploadZip(ctx, publishedWorkspace(ws), namedPackage(t, name, withScript))
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Blocked {
		t.Fatalf("test package did not validate: %+v", res.Report.Findings)
	}
	id, _ := res.Skill.ID.Value()
	skillID, _ := id.(string)

	if _, err := pool.Exec(ctx, "UPDATE search_documents SET enrichment_status = 'enriched' WHERE skill_id = $1", res.Skill.ID); err != nil {
		t.Fatal(err)
	}
	return skillID
}

func namedPackage(t *testing.T, name string, withScript bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"SKILL.md": "---\nname: " + name + "\ndescription: Reports on " + name +
			".\nlicense: MIT\n---\n\nUse it like this.\n",
	}
	if withScript {
		files["scripts/run.py"] = "print('hello')\n"
	}
	for path, body := range files {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (s packageStore) Put(_ context.Context, key string, data []byte) error {
	s[key] = data
	return nil
}

type searchResult struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`

	Rank     *float64 `json:"rank"`
	RankNote string   `json:"rank_note"`
	Tier     struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"tier"`
	Category struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Note  string `json:"note"`
	} `json:"category"`
	Risk struct {
		ScanStatus      string                               `json:"scan_status"`
		Level           string                               `json:"level"`
		Warnings        int                                  `json:"warnings"`
		Disclosures     []struct{ Code, Label, Note string } `json:"disclosures"`
		HasExternalURLs bool                                 `json:"has_external_urls"`
	} `json:"risk"`
	Dependencies  []string `json:"dependencies"`
	Compatibility struct {
		SpecValidation struct{ Value, Label, Note string } `json:"spec_validation"`
		Capability     struct{ Value, Label, Note string } `json:"capability"`
		Runtime        struct{ Value, Label, Note string } `json:"runtime"`
	} `json:"compatibility"`
	VerifiedAt        string `json:"verified_at"`
	MatchReason       string `json:"match_reason"`
	MatchReasonSource string `json:"match_reason_source"`
}

type searchBody struct {
	Query           string         `json:"query"`
	Results         []searchResult `json:"results"`
	Degraded        bool           `json:"degraded"`
	DegradedReason  string         `json:"degraded_reason"`
	PartialIndex    bool           `json:"partial_index"`
	NoResults       bool           `json:"no_results"`
	QuerySuggestion string         `json:"query_suggestion"`
	FilteredOut     bool           `json:"filtered_out"`
	Limit           int            `json:"limit"`
	Truncated       bool           `json:"truncated"`
	Total           int            `json:"total"`
}

func (c *client) search(t *testing.T, path string) searchBody {
	t.Helper()
	resp, err := c.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: got %d", path, resp.StatusCode)
	}
	var out searchBody
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (b searchBody) ids() []string {
	ids := make([]string, 0, len(b.Results))
	for _, r := range b.Results {
		ids = append(ids, r.SkillID)
	}
	return ids
}

func TestForkCatalogSkillIntoCallerWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-fork")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "catalog-invoice-parser")
	publishedVer := seedSkillVersion(t, pool, curator.workspaceID, published)

	alice := a.login(t, "alice-fork")
	fork := postFork(t, alice, published, http.StatusCreated)

	if fork.ForkedFromSkillID == nil || *fork.ForkedFromSkillID != published {
		t.Fatalf("forked_from_skill_id = %v, want %s", fork.ForkedFromSkillID, published)
	}
	if fork.ForkedFromVersionID == nil || *fork.ForkedFromVersionID != publishedVer {
		t.Fatalf("forked_from_version_id = %v, want %s", fork.ForkedFromVersionID, publishedVer)
	}

	if ids := alice.skillIDs(t, "/skills"); !contains(ids, fork.SkillID) {
		t.Fatalf("fork missing from the forker's own skills: %v", ids)
	}
	if ids := curator.skillIDs(t, "/skills"); contains(ids, fork.SkillID) {
		t.Fatal("fork landed in the catalog workspace instead of the caller's")
	}

	if ids := curator.skillIDs(t, "/skills"); !contains(ids, published) {
		t.Fatal("forking removed or altered the catalog original")
	}
}

func TestForkOfAnotherUsersPrivateSkillStillNotFound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	owner := a.login(t, "owner-private-fork")
	secret := seedSkill(t, pool, owner.workspaceID, "private-invoice-parser")
	seedSkillVersion(t, pool, owner.workspaceID, secret)

	stranger := a.login(t, "stranger-private-fork")
	postFork(t, stranger, secret, http.StatusNotFound)

	if ids := stranger.skillIDs(t, "/skills"); len(ids) != 0 {
		t.Fatalf("a refused fork still created skills: %v", ids)
	}
}

type forkBody struct {
	SkillID             string  `json:"skill_id"`
	ForkedFromSkillID   *string `json:"forked_from_skill_id"`
	ForkedFromVersionID *string `json:"forked_from_version_id"`
}

func postFork(t *testing.T, c *client, skillID string, wantStatus int) forkBody {
	t.Helper()
	resp, err := c.Post(c.base+"/skills/"+skillID+"/fork", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST fork %s: want %d, got %d", skillID, wantStatus, resp.StatusCode)
	}
	var out forkBody
	if wantStatus == http.StatusCreated {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestZeroHitLegDoesNotParticipateInFusion(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-rrf")
	markCatalog(t, pool, curator.workspaceID)
	near := seedSkill(t, pool, curator.workspaceID, "quenchable ledger reconciler")
	far := seedSkill(t, pool, curator.workspaceID, "quenchable image rotator")
	seedEmbedding(t, pool, near, 7)

	seedBlendedEmbedding(t, pool, far, 7, 900, 0.5)

	a := newAPIWithLLM(t, pool, stubLLM(t, 7, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=%E5%B8%B3%E5%8B%99%E5%B0%8D%E5%B8%B3")
	if body.Degraded {
		t.Fatalf("hybrid path reported degraded: %q", body.DegradedReason)
	}
	ids := body.ids()
	if len(ids) == 0 {
		t.Fatal("a zero-hit FTS leg suppressed the vector leg's results entirely")
	}
	if ids[0] != near {
		t.Fatalf("vector ranking not preserved: got %v, want %s first", ids, near)
	}
	if !contains(ids, far) {
		t.Fatalf("vector leg dropped a candidate: %v", ids)
	}
}

func TestPublicHybridSearchDoesNotLeakPrivateWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-scope-hybrid")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "borogove ledger reconciler")

	owner := a.login(t, "owner-scope-hybrid")
	private := seedSkill(t, pool, owner.workspaceID, "borogove ledger reconciler")

	seedEmbedding(t, pool, published, 7)
	seedEmbedding(t, pool, private, 7)

	hybrid := newAPIWithLLM(t, pool, stubLLM(t, 7, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: hybrid.URL}
	body := anon.search(t, "/api/skills/search?q=borogove+ledger")
	if body.Degraded {
		t.Fatalf("meant to exercise the hybrid path, got the degraded one: %q", body.DegradedReason)
	}

	ids := body.ids()
	if !contains(ids, published) {
		t.Fatalf("the catalog document did not come back at all, so nothing was proved: %v", ids)
	}
	if contains(ids, private) {
		t.Fatalf("public search answered an anonymous request with a private workspace's skill: %v", ids)
	}
}

func TestFTSWidensCandidatesWithoutTakingOverRanking(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-ranking")
	markCatalog(t, pool, curator.workspaceID)

	lexical := seedSkill(t, pool, curator.workspaceID, "borogove ledger reconciler")
	semantic := seedSkill(t, pool, curator.workspaceID, "mome rath invoice matcher")
	seedBlendedEmbedding(t, pool, lexical, 21, 900, 0.4)
	seedEmbedding(t, pool, semantic, 21)

	a := newAPIWithLLM(t, pool, stubLLM(t, 21, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=borogove")
	ids := body.ids()
	if len(ids) != 2 {
		t.Fatalf("want both documents in the candidate set, got %v", ids)
	}
	if ids[0] != semantic {
		t.Fatalf("lexical rank overrode vector ranking: got %v, want %s first", ids, semantic)
	}

	if ids[1] != lexical {
		t.Fatalf("FTS candidate expansion dropped its own hit: %v", ids)
	}

	if body.Results[0].Rank == nil || body.Results[1].Rank == nil {
		t.Fatalf("a fully ranked page reported a null rank: %+v", body.Results)
	}
	if *body.Results[0].Rank <= *body.Results[1].Rank {
		t.Fatalf("rank does not follow the ordering: %+v", body.Results)
	}

	for _, r := range body.Results {
		if *r.Rank < 0 || *r.Rank > 1 {
			t.Fatalf("rank %v is outside the documented 0..1", *r.Rank)
		}
	}
}

func TestOffTopicQueryIsRefusedWithASuggestion(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-threshold")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "jubjub ledger reconciler")
	seedEmbedding(t, pool, published, 33)

	a := newAPIWithLLM(t, pool, stubLLM(t, 700, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=%E6%89%8B%E6%B2%96%E5%92%96%E5%95%A1%E7%A3%A8%E8%B1%86%E7%B2%97%E7%B4%B0")
	if len(body.Results) != 0 {
		t.Fatalf("off-topic query returned %d results: %v", len(body.Results), body.ids())
	}
	if !body.NoResults {
		t.Fatal("empty result set was not reported as no_results")
	}
	requireInterfaceLanguage(t, "the no-results query suggestion", body.QuerySuggestion)

	if body.Degraded {
		t.Fatalf("a refusal was reported as a degradation: %q", body.DegradedReason)
	}
}

func TestRelevantQueryIsNotRefusedByTheCutOff(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-threshold-ok")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "slithy ledger reconciler")

	seedBlendedEmbedding(t, pool, published, 44, 900, 0.3)

	a := newAPIWithLLM(t, pool, stubLLM(t, 44, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=%E5%B8%B3%E5%8B%99%E5%B0%8D%E5%B8%B3")
	if !contains(body.ids(), published) {
		t.Fatalf("the cut-off refused a genuine match: %v", body.ids())
	}
	if body.NoResults {
		t.Fatal("a search with results reported no_results")
	}
}

func TestSearchDegradesToFTSWhenEmbeddingFails(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-degrade")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "quixotical ledger reconciler")
	seedEmbedding(t, pool, published, 3)

	a := newAPIWithLLM(t, pool, stubLLM(t, -1, ""))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=quixotical")
	if !body.Degraded {
		t.Fatal("embed failure was not reported as degraded")
	}
	if body.DegradedReason == "" {
		t.Fatal("degraded answer carries no reason")
	}
	if !contains(body.ids(), published) {
		t.Fatalf("degradation lost the lexical answer too: %v", body.ids())
	}

	if got := body.Results[0].MatchReasonSource; got != "template" {
		t.Fatalf("match_reason_source = %q, want template", got)
	}

	for _, r := range body.Results {
		if r.Rank != nil {
			t.Fatalf("degraded answer reported a similarity it never computed: %v", *r.Rank)
		}
		if r.RankNote == "" {
			t.Fatal("null rank with no explanation of what ordered the page")
		}
	}
}

func TestSearchResultsCarryTheDISC002Columns(t *testing.T) {
	pool := requireDB(t)

	a := newAPI(t, pool)
	curator := a.login(t, "curator-facets")
	markCatalog(t, pool, curator.workspaceID)

	importPackage(t, pool, a.packages, curator, "callooh-callay-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}
	body := anon.search(t, "/api/skills/search?q=callooh")
	if len(body.Results) != 1 {
		t.Fatalf("import did not become searchable: %+v", body.Results)
	}
	got := body.Results[0]

	if got.Name == "" || got.Summary == "" {
		t.Fatalf("result is missing name or summary: %+v", got)
	}

	if got.Tier.Value != "indexed" || got.Tier.Label == "" {
		t.Fatalf("tier = %+v, want indexed with its copy", got.Tier)
	}

	if got.Risk.ScanStatus != "scanned" {
		t.Fatalf("risk scan_status = %q, want scanned", got.Risk.ScanStatus)
	}
	if !hasDisclosureCode(got.Risk.Disclosures, "script-file") {
		t.Fatalf("a package containing a script disclosed none: %+v", got.Risk.Disclosures)
	}
	if got.Risk.Level == "none" {
		t.Fatalf("risk level = none for a package with disclosures: %+v", got.Risk)
	}

	if got.Compatibility.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q for an accepted import", got.Compatibility.SpecValidation.Value)
	}
	if got.Compatibility.Capability.Value != "unverified" || got.Compatibility.Runtime.Value != "unverified" {
		t.Fatalf("sandbox axes claimed a verdict before M2: %+v", got.Compatibility)
	}

	if got.VerifiedAt == "" {
		t.Fatal("no verification time on a result with a saved version")
	}

	if got.Dependencies == nil {
		t.Fatal("dependencies omitted; an absent list reads as 'none'")
	}
}

func TestUnversionedSkillDoesNotClaimSpecValidation(t *testing.T) {
	pool := requireDB(t)

	a := newAPI(t, pool)
	curator := a.login(t, "curator-noversion")
	markCatalog(t, pool, curator.workspaceID)
	seedSkill(t, pool, curator.workspaceID, "frumious bandersnatch tracker")

	anon := &client{Client: http.DefaultClient, base: a.URL}
	body := anon.search(t, "/api/skills/search?q=frumious")
	if len(body.Results) != 1 {
		t.Fatalf("seeded skill not searchable: %v", body.ids())
	}
	got := body.Results[0]
	if got.Compatibility.SpecValidation.Value != "unverified" {
		t.Fatalf("spec_validation = %q for a skill with nothing saved", got.Compatibility.SpecValidation.Value)
	}
	if got.VerifiedAt != "" {
		t.Fatalf("verified_at = %q for content that was never imported", got.VerifiedAt)
	}

	if got.Risk.ScanStatus != "unavailable" {
		t.Fatalf("risk scan_status = %q for a row with no scan", got.Risk.ScanStatus)
	}
}

func TestSearchWithoutLLMServiceIsDegradedButAnswers(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-nollm")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "vorpal ledger reconciler")

	anon := &client{Client: http.DefaultClient, base: a.URL}
	body := anon.search(t, "/api/skills/search?q=vorpal")
	if !body.Degraded {
		t.Fatal("a search with no embedding service was not marked degraded")
	}
	if !contains(body.ids(), published) {
		t.Fatalf("degraded search returned nothing: %v", body.ids())
	}
}

func TestPartialIndexIsReportedSeparatelyFromDegradation(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-partial")
	markCatalog(t, pool, curator.workspaceID)
	enriched := seedSkill(t, pool, curator.workspaceID, "mimsy ledger reconciler")
	seedEmbedding(t, pool, enriched, 55)

	pending := seedSkill(t, pool, curator.workspaceID, "mimsy invoice matcher")
	if _, err := pool.Exec(context.Background(), "UPDATE search_documents SET enrichment_status = 'pending' WHERE skill_id = $1", mustUUID(t, pending)); err != nil {
		t.Fatal(err)
	}

	a := newAPIWithLLM(t, pool, stubLLM(t, 55, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=mimsy")
	if body.Degraded {
		t.Fatalf("index coverage was reported as an outage: %q", body.DegradedReason)
	}

	if contains(body.ids(), pending) {
		t.Fatalf("a document with no metadata reached the public page: %v", body.ids())
	}
	if body.PartialIndex {
		t.Fatalf("a page with no unranked row reported partial_index: %v", body.ids())
	}

	seedEmbedding(t, pool, pending, 55)
	body = anon.search(t, "/api/skills/search?q=mimsy")
	if !contains(body.ids(), pending) || body.PartialIndex {
		t.Fatalf("an embedded document must be on the page and ranked: %v partial=%v", body.ids(), body.PartialIndex)
	}
}

func TestMatchReasonsAreLabelledByProvenance(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-reasons")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "frumious ledger reconciler")
	seedEmbedding(t, pool, published, 11)

	model := newAPIWithLLM(t, pool, stubLLM(t, 11, "it reconciles ledgers, which is what you asked for"))
	anon := &client{Client: http.DefaultClient, base: model.URL}
	body := anon.search(t, "/api/skills/search?q=frumious")
	if len(body.Results) == 0 {
		t.Fatal("no results to explain")
	}
	if got := body.Results[0].MatchReasonSource; got != "model" {
		t.Fatalf("match_reason_source = %q, want model", got)
	}
	if body.Results[0].MatchReason == "" {
		t.Fatal("model-sourced result carries no reason")
	}

	tmpl := newAPIWithLLM(t, pool, stubLLM(t, 11, ""))
	anon = &client{Client: http.DefaultClient, base: tmpl.URL}
	body = anon.search(t, "/api/skills/search?q=frumious")
	if len(body.Results) == 0 {
		t.Fatal("no results to explain on the template path")
	}
	if got := body.Results[0].MatchReasonSource; got != "template" {
		t.Fatalf("match_reason_source = %q, want template", got)
	}

	requireInterfaceLanguage(t, "the template match reason", body.Results[0].MatchReason)
}

func TestBlankQueryReturnsNoResults(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-blank")
	markCatalog(t, pool, curator.workspaceID)
	seedSkill(t, pool, curator.workspaceID, "slithy ledger reconciler")

	a := newAPIWithLLM(t, pool, stubLLM(t, 5, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}
	for _, q := range []string{"", "%20", "a"} {
		body := anon.search(t, "/api/skills/search?q="+q)
		if len(body.Results) != 0 {
			t.Errorf("q=%q produced %d results, want none", q, len(body.Results))
		}

		if !body.NoResults || body.QuerySuggestion == "" {
			t.Errorf("q=%q returned an empty list with no explanation", q)
		}
	}

	resp, err := anon.Get(a.URL + "/api/skills/search")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a search with no q parameter: got %d, want 400", resp.StatusCode)
	}
}

func TestFiltersNarrowOnRealEvidenceIncludingTheDegradedPath(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filters")
	markCatalog(t, pool, curator.workspaceID)

	scripted := importPackage(t, pool, a.packages, curator, "uffish-scripted-reporter", true)
	plain := importPackage(t, pool, a.packages, curator, "uffish-plain-reporter", false)

	unscanned := seedSkill(t, pool, curator.workspaceID, "uffish unscanned reporter")

	anon := &client{Client: http.DefaultClient, base: a.URL}

	all := anon.search(t, "/api/skills/search?q=uffish")
	if !all.Degraded {
		t.Fatal("no LLM service configured but the answer was not marked degraded")
	}
	for _, want := range []string{scripted, plain, unscanned} {
		if !contains(all.ids(), want) {
			t.Fatalf("unfiltered search missed %s: %v", want, all.ids())
		}
	}

	tests := []struct {
		name  string
		path  string
		want  []string
		notIn []string
	}{
		{"script=yes", "&script=yes", []string{scripted}, []string{plain, unscanned}},

		{"script=no", "&script=no", []string{plain}, []string{scripted, unscanned}},
		{"validation=passed", "&validation=passed", []string{scripted, plain}, []string{unscanned}},
		{"validation=unverified", "&validation=unverified", []string{unscanned}, []string{scripted, plain}},

		{"combined", "&script=no&validation=passed", []string{plain}, []string{scripted, unscanned}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := anon.search(t, "/api/skills/search?q=uffish"+tc.path)
			for _, want := range tc.want {
				if !contains(body.ids(), want) {
					t.Errorf("filter dropped a matching skill %s: %v", want, body.ids())
				}
			}
			for _, unwanted := range tc.notIn {
				if contains(body.ids(), unwanted) {
					t.Errorf("filter kept a non-matching skill %s: %v", unwanted, body.ids())
				}
			}

			if body.NoResults || body.FilteredOut {
				t.Errorf("a page with results reported an empty state: %+v", body)
			}
		})
	}
}

func TestFiltersOnTheHybridPathRemoveRowsWithoutReranking(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filters-hybrid")
	markCatalog(t, pool, curator.workspaceID)

	scripted := importPackage(t, pool, a.packages, curator, "outgrabe-scripted-reporter", true)
	plain := importPackage(t, pool, a.packages, curator, "outgrabe-plain-reporter", false)

	seedEmbedding(t, pool, scripted, 61)
	seedBlendedEmbedding(t, pool, plain, 61, 900, 0.5)

	hybrid := newAPIWithLLM(t, pool, stubLLM(t, 61, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: hybrid.URL}

	all := anon.search(t, "/api/skills/search?q=outgrabe")
	if all.Degraded {
		t.Fatalf("hybrid path reported degraded: %q", all.DegradedReason)
	}
	if len(all.Results) != 2 || all.Results[0].SkillID != scripted {
		t.Fatalf("unexpected unfiltered page: %v", all.ids())
	}
	plainRank := all.Results[1].Rank
	if plainRank == nil {
		t.Fatal("hybrid page returned a null rank for an embedded document")
	}

	filtered := anon.search(t, "/api/skills/search?q=outgrabe&script=no")
	if got := filtered.ids(); len(got) != 1 || got[0] != plain {
		t.Fatalf("script=no on the hybrid path returned %v, want just %s", got, plain)
	}

	if got := filtered.Results[0].Rank; got == nil || *got != *plainRank {
		t.Fatalf("rank changed under filtering: %v, want %v", got, *plainRank)
	}
}

func TestFilteredToEmptyIsNotTheNoResultsRefusal(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filtered-empty")
	markCatalog(t, pool, curator.workspaceID)
	importPackage(t, pool, a.packages, curator, "galumph-scripted-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	filtered := anon.search(t, "/api/skills/search?q=galumph&script=no")
	if len(filtered.Results) != 0 {
		t.Fatalf("script=no kept a scripted package: %v", filtered.ids())
	}
	if !filtered.FilteredOut {
		t.Fatal("a page emptied by its filters did not report filtered_out")
	}
	if filtered.NoResults {
		t.Fatal("a page emptied by its filters was reported as no_results as well")
	}

	if filtered.QuerySuggestion != "" {
		t.Fatalf("filtered-out answer carried the query suggestion: %q", filtered.QuerySuggestion)
	}

	refused := anon.search(t, "/api/skills/search?q=whiffling&script=no")
	if !refused.NoResults {
		t.Fatalf("an unmatched query was not refused: %+v", refused)
	}
	requireInterfaceLanguage(t, "the query suggestion on the unmatched-query refusal", refused.QuerySuggestion)
	if refused.FilteredOut {
		t.Fatal("a query that matched nothing blamed the filters for the empty page")
	}
}

func TestFilterDimensionsWithoutDataAreRejectedNotIgnored(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filter-reject")
	markCatalog(t, pool, curator.workspaceID)
	importPackage(t, pool, a.packages, curator, "beamish-scripted-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, q := range []string{
		"&category=unassigned",

		"&agent=claude",
		"&tier=external",

		"&mcp=no",
		"&script=maybe",
		"&validation=failed",
		"&script=",
		"&validation=",
		"&agent=",
		"&tier=",
		"&category=",
	} {
		if got := anon.status(t, http.MethodGet, "/api/skills/search?q=beamish"+q); got != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 — an unusable filter must not be silently dropped", q, got)
		}
		if got := anon.status(t, http.MethodGet, "/api/skills/catalog?"+strings.TrimPrefix(q, "&")); got != http.StatusBadRequest {
			t.Errorf("catalog %s: got %d, want 400 — both handwritten routes must enforce the contract", q, got)
		}
	}

	if body := anon.search(t, "/api/skills/search?q=beamish&script=yes"); len(body.Results) != 1 {
		t.Fatalf("a supported filter was rejected too: %+v", body)
	}
}

func setCategory(t *testing.T, pool *pgxpool.Pool, skillID, category string) {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),

		"UPDATE skills SET category = $2, category_source = 'curated' WHERE id = $1", id, category,
	); err != nil {
		t.Fatal(err)
	}
}

func TestCategoryFiltersTheCatalogAndNamesTheAbsence(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-category")
	markCatalog(t, pool, curator.workspaceID)

	shelved := importPackage(t, pool, a.packages, curator, "borogove-deduper", false)
	unclassified := importPackage(t, pool, a.packages, curator, "borogove-reporter", false)
	setCategory(t, pool, shelved, "data")

	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, path := range []string{
		"/api/skills/search?q=borogove&category=data",
		"/api/skills/catalog?category=data",
	} {
		got := anon.search(t, path)
		if ids := got.ids(); len(ids) != 1 || ids[0] != shelved {
			t.Fatalf("%s returned %v, want just the data-shelved skill %s", path, ids, shelved)
		}
		if c := got.Results[0].Category; c.Value != "data" || c.Label != "資料" {
			t.Fatalf("%s: the shelf did not survive the read: %+v", path, c)
		}
	}

	for _, path := range []string{
		"/api/skills/search?q=borogove&category=writing",
		"/api/skills/catalog?category=writing",
	} {
		if ids := anon.search(t, path).ids(); len(ids) != 0 {
			t.Fatalf("%s returned %v; nothing on this catalogue is shelved as writing", path, ids)
		}
	}

	all := anon.search(t, "/api/skills/catalog")
	var found bool
	for _, r := range all.Results {
		if r.SkillID != unclassified {
			continue
		}
		found = true
		if r.Category.Value != "unassigned" || r.Category.Label != "尚未定值" {
			t.Errorf("an unclassified row rendered as %+v, want unassigned/尚未定值 (設計 §2.9)", r.Category)
		}
		if r.Category.Note == "" {
			t.Error("the absence was rendered without saying why it is absent (05 R-19)")
		}
	}
	if !found {
		t.Fatalf("the unclassified skill fell out of the unfiltered catalogue: %v", all.ids())
	}

	var shelvedDetail, plainDetail detail
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+shelved, &shelvedDetail); code != http.StatusOK {
		t.Fatalf("detail for the shelved skill: got %d", code)
	}
	if shelvedDetail.Category.Value != "data" || shelvedDetail.Category.Label != "資料" {
		t.Errorf("detail lost the shelf: %+v", shelvedDetail.Category)
	}
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+unclassified, &plainDetail); code != http.StatusOK {
		t.Fatalf("detail for the unclassified skill: got %d", code)
	}
	if plainDetail.Category.Value != "unassigned" || plainDetail.Category.Label != "尚未定值" {
		t.Errorf("detail rendered an unclassified skill as %+v, want unassigned/尚未定值", plainDetail.Category)
	}
}

func TestAnImportedSkillArrivesWithoutACategory(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "owner-import-category")
	imported := importPackage(t, pool, a.packages, owner, "slithy-importer", false)

	var stored *string
	if err := pool.QueryRow(context.Background(),
		"SELECT category FROM skills WHERE id = $1", mustUUID(t, imported),
	).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Fatalf("import assigned category %q; 05 R-19 has not decided where one comes from", *stored)
	}

	var got detail
	if code := getJSON(t, owner.Client, a.URL+"/api/skills/"+imported, &got); code != http.StatusOK {
		t.Fatalf("owner GET /api/skills/{id}: got %d", code)
	}
	if got.Category.Value != "unassigned" || got.Category.Label != "尚未定值" {
		t.Errorf("an imported skill rendered as %+v, want unassigned/尚未定值", got.Category)
	}
}

func restrict(t *testing.T, pool *pgxpool.Pool, skillID string) {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET access_restriction = 'license-review' WHERE id = $1", id,
	); err != nil {
		t.Fatal(err)
	}
}

func TestLicensingHoldClosesTheMaterialsAndKeepsTheListing(t *testing.T) {
	pool := requireDB(t)

	a := newAPI(t, pool)
	curator := a.login(t, "curator-hold")
	markCatalog(t, pool, curator.workspaceID)

	held := importPackage(t, pool, a.packages, curator, "brillig-restricted-writer", true)
	free := importPackage(t, pool, a.packages, curator, "brillig-open-writer", true)
	restrict(t, pool, held)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=brillig")
	names := map[string]bool{}
	for _, r := range body.Results {
		names[r.Name] = true
	}
	if !names["brillig-restricted-writer"] || !names["brillig-open-writer"] {
		t.Fatalf("search dropped a skill it should still list: %+v", body.Results)
	}

	code, detail := anon.doJSON(t, http.MethodGet, "/api/skills/"+held, "")
	if code != http.StatusOK {
		t.Fatalf("detail of a held skill answered %d; the listing must stay usable", code)
	}
	rest, _ := detail["access_restriction"].(map[string]any)
	if rest == nil || rest["reason"] != "license-review" {
		t.Fatalf("detail did not disclose the hold: %+v", detail["access_restriction"])
	}
	if rest["note"] == "" || detail["summary"] == "" {
		t.Fatalf("hold left the reader with nothing: note=%v summary=%v", rest["note"], detail["summary"])
	}

	code, files := anon.doJSON(t, http.MethodGet, "/api/skills/"+held+"/files", "")
	if code != http.StatusForbidden {
		t.Fatalf("GET /files on a held skill answered %d, want 403", code)
	}
	if msg, _ := files["error"].(string); msg == "" {
		t.Fatal("the refusal carried no reason; a bare 403 is indistinguishable from a bug")
	}

	code, open := anon.doJSON(t, http.MethodGet, "/api/skills/"+free, "")
	if code != http.StatusOK || open["access_restriction"] != nil {
		t.Fatalf("the hold reached a skill it was not applied to: code=%d %+v", code, open["access_restriction"])
	}
	if code := anon.status(t, http.MethodGet, "/api/skills/"+free+"/files"); code != http.StatusOK {
		t.Fatalf("GET /files on an unrestricted skill answered %d, want 200", code)
	}
}

func TestARunOnHeldMaterialsIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-held")
	restrict(t, pool, f.skillID)

	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+`"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on held materials: got %d, want 422", code)
	}
	if !strings.Contains(view.Error, "license") {
		t.Errorf("refusal = %q, want it to say the licence review is why", view.Error)
	}
}

func hasDisclosureCode(list []struct{ Code, Label, Note string }, code string) bool {
	for _, d := range list {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestATruncatedSearchSaysHowManyMatchedAndNotJustThatThereWereMore(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	anon := &client{Client: http.DefaultClient, base: a.URL}
	curator := a.login(t, "curator-total")
	markCatalog(t, pool, curator.workspaceID)
	for i := range 5 {
		seedSkill(t, pool, curator.workspaceID, fmt.Sprintf("uffish thing %d", i))
	}

	full := anon.search(t, "/api/skills/search?q=uffish&limit=100")
	if full.Truncated {
		t.Fatalf("five documents under a cap of 100 should not truncate: %+v", full)
	}
	if full.Total != len(full.Results) {
		t.Errorf("an untruncated page must account for itself exactly: total=%d, rows=%d",
			full.Total, len(full.Results))
	}
	if full.Total == 0 {
		t.Fatal("nothing matched, so this test proves nothing about the count")
	}

	cut := anon.search(t, "/api/skills/search?q=uffish&limit=1")
	if !cut.Truncated || len(cut.Results) != 1 {
		t.Fatalf("limit=1 over %d matches should truncate to one row: %+v", full.Total, cut)
	}

	if cut.Total != full.Total {
		t.Errorf("the total describes the matches, not the page: cut=%d, full=%d",
			cut.Total, full.Total)
	}
	if cut.Total <= len(cut.Results) {
		t.Errorf("a truncated page reporting total=%d for %d rows says nothing was cut",
			cut.Total, len(cut.Results))
	}
}

func TestSearchLimitOutsideTheSchemaIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "owner-search-limit")
	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, ep := range []struct {
		name string
		path string
		cl   *client
	}{
		{"public", "/api/skills/search?q=uffish", anon},
		{"workspace", "/skills/search?q=uffish", owner},
	} {

		for _, ok := range []string{"", "&limit=1", "&limit=100", "&limit=20"} {
			if got := ep.cl.status(t, http.MethodGet, ep.path+ok); got != http.StatusOK {
				t.Errorf("%s search%q: got %d, want 200", ep.name, ok, got)
			}
		}

		for _, bad := range []string{"&limit=0", "&limit=-1", "&limit=101", "&limit=500", "&limit=abc", "&limit=1.5", "&limit="} {
			if got := ep.cl.status(t, http.MethodGet, ep.path+bad); got != http.StatusBadRequest {
				t.Errorf("%s search%q: got %d, want 400", ep.name, bad, got)
			}
		}
	}
}

func TestAForkCarriesTheCategoryOfWhatItWasForkedFrom(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "curator")
	makeCatalog(t, pool, owner.workspaceID)
	skillID, _ := packagedSkill(t, a, pool, owner, "shelved-skill")
	setCategory(t, pool, skillID, "data")

	forker := a.login(t, "forker")
	code, body := postJSON(t, forker, "/skills/"+skillID+"/fork", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, body)
	}
	forkID, _ := body["skill_id"].(string)

	var got *string
	if err := pool.QueryRow(context.Background(),
		"SELECT category FROM skills WHERE id=$1", mustUUID(t, forkID)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != "data" {
		t.Fatalf("the fork's category is %v; the shelf was lost in the copy (0053)", got)
	}

	var forkDetail detail
	if code := getJSON(t, forker.Client, a.URL+"/api/skills/"+forkID, &forkDetail); code != http.StatusOK {
		t.Fatalf("GET fork detail: got %d", code)
	}
	if forkDetail.Category.Value != "data" || forkDetail.Category.Label != "資料" {
		t.Errorf("fork detail category = %+v, want data/資料", forkDetail.Category)
	}
}
