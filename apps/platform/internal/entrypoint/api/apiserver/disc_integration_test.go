// DISC-001 / DISC-002 / WS-001 database-backed tests. Shared harness (TestMain,
// migrate, requireDB, login, seedSkill) lives in authz_integration_test.go.
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

// --- helpers ---------------------------------------------------------------

// requireInterfaceLanguage asserts that a string the page renders verbatim
// actually reaches the wire in the interface language. The front end lays these
// out and translates nothing (WorkspaceRuns.tsx:65-83), so an English string
// here is an English string on a Traditional Chinese search page.
//
// It is a Han check rather than a text match because the wording is expected to
// change and the language is not. Every other assertion on this copy in this
// file tests `!= ""`, which is how three English sentences survived M1: a
// non-empty check cannot tell the two languages apart. The unit-level twin,
// with the stricter no-Latin-prose rule, is in
// internal/skill/discovery/reason_test.go.
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

// seedSkillVersion gives a seeded skill the immutable version a fork copies
// from. Without one, a fork fails for lack of content and a scope test would
// pass for the wrong reason.
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

// seedEmbedding fills the vector leg for one document. axis picks which
// dimension carries the signal, so two documents can be made near or far from a
// query vector without calling a model.
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

// seedBlendedEmbedding places a document at a chosen cosine similarity to the
// query axis. Needed because a document on a different unit axis sits at cosine
// distance 1.0, which is past catalog.MaxCosineDistance — that is a document the
// cut-off is supposed to drop, so it cannot stand in for a merely-weaker match.
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

// stubLLM stands in for the internal Python service. embedAxis < 0 makes /embed
// fail, which is the degradation trigger the tests care about.
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

// importPackage runs a package through the real import pipeline — validation,
// object store, version row, search projection — so the facets a result row
// reads were written the way production writes them. No LLM: the scan-derived
// facets must not depend on one.
//
// withScript decides whether the package carries one, which is the evidence the
// DISC-003 script filter reads. Returns the new skill's id so a test can seed an
// embedding for it and exercise the hybrid path against real imported rows.
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
	return skillID
}

// namedPackage is demoPackage under a caller-chosen skill name, so a search
// query can single it out. With withScript it carries one for the same reason
// demoPackage does — a package with nothing to disclose cannot show that
// disclosures travel — and without it, it is the other side of the DISC-003
// script filter: a scanned package the scan found no script in, which is a
// different answer from a package nobody scanned.
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

// Put returns the packageStore's write half, which import needs and the
// read-only detail view does not.
func (s packageStore) Put(_ context.Context, key string, data []byte) error {
	s[key] = data
	return nil
}

type searchResult struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Pointer, not float64: null is a distinct answer from 0 here, and decoding
	// it into a value type would silently turn "never measured" into "measured
	// as far away as possible".
	Rank     *float64 `json:"rank"`
	RankNote string   `json:"rank_note"`
	Tier     struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"tier"`
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

// --- WS-001: forking public catalog content ---------------------------------

// WS-001: the catalog exists to be forked. Before the catalog scope was added
// to the fork read, every fork of a public skill answered 404 because the read
// only looked in the caller's own workspace.
func TestForkCatalogSkillIntoCallerWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-fork")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "catalog-invoice-parser")
	publishedVer := seedSkillVersion(t, pool, curator.workspaceID, published)

	alice := a.login(t, "alice-fork")
	fork := postFork(t, alice, published, http.StatusCreated)

	// Provenance: the fork points back at the exact origin skill and version
	// it was taken from (WS-001, iron rule 4 — nothing was rewritten in place).
	if fork.ForkedFromSkillID == nil || *fork.ForkedFromSkillID != published {
		t.Fatalf("forked_from_skill_id = %v, want %s", fork.ForkedFromSkillID, published)
	}
	if fork.ForkedFromVersionID == nil || *fork.ForkedFromVersionID != publishedVer {
		t.Fatalf("forked_from_version_id = %v, want %s", fork.ForkedFromVersionID, publishedVer)
	}

	// The fork landed in the caller's workspace, not the catalog's.
	if ids := alice.skillIDs(t, "/skills"); !contains(ids, fork.SkillID) {
		t.Fatalf("fork missing from the forker's own skills: %v", ids)
	}
	if ids := curator.skillIDs(t, "/skills"); contains(ids, fork.SkillID) {
		t.Fatal("fork landed in the catalog workspace instead of the caller's")
	}
	// The origin is untouched: a fork copies rows, never mutates the source.
	if ids := curator.skillIDs(t, "/skills"); !contains(ids, published) {
		t.Fatal("forking removed or altered the catalog original")
	}
}

// WS-006: widening the fork read to the catalog must not widen it to everyone
// else's private workspaces. The private skill here has a version, so a 404 can
// only come from the scope check and not from missing content.
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

// --- DISC-001: hybrid retrieval and its degradation path --------------------

// ADR-013 定案調整 3: a leg that matched nothing must not take part in the
// fusion. Here the FTS leg matches nothing at all (the query shares no word
// with either document) and the ranking has to come from the vector leg alone —
// which is exactly the cross-language case the vector leg exists for.
func TestZeroHitLegDoesNotParticipateInFusion(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-rrf")
	markCatalog(t, pool, curator.workspaceID)
	near := seedSkill(t, pool, curator.workspaceID, "quenchable ledger reconciler")
	far := seedSkill(t, pool, curator.workspaceID, "quenchable image rotator")
	seedEmbedding(t, pool, near, 7)
	// Weaker but still inside the DISC-005 cut-off, so this test keeps measuring
	// fusion behaviour rather than accidentally measuring the threshold.
	seedBlendedEmbedding(t, pool, far, 7, 900, 0.5)

	// A fresh API whose stub embeds every query onto the same axis as `near`.
	a := newAPIWithLLM(t, pool, stubLLM(t, 7, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	// "帳務對帳" shares no token with either English document, so the FTS leg
	// returns zero rows.
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

// CORE-006 and iron rule 3: the public scope is fixed inside the retrieval SQL,
// and the vector leg has to carry it too.
//
// Every scope test that existed ran against the degraded path — they were
// written with no LLM configured, and FTS-only search is a different statement
// with its own copy of the predicate. Deleting BOTH `AND w.is_catalog` clauses
// from PublicHybridSearchSkills left the entire suite green (M1 audit,
// 2026-08-24). That is an anonymous, unauthenticated read of every private
// workspace in the database, one deleted line away.
//
// The private document is deliberately identical to the published one in name
// and in embedding axis. If it differed in either, it could be excluded by
// ranking rather than by scope and the test would pass for the wrong reason.
func TestPublicHybridSearchDoesNotLeakPrivateWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-scope-hybrid")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "borogove ledger reconciler")

	owner := a.login(t, "owner-scope-hybrid") // not marked catalog
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

// golden-query-set.md §3.7: the BM25 leg widens the candidate set and nothing
// more. Here the lexical leg's best hit is the vector leg's worst, so if the FTS
// rank still carried any ordering weight — as it did under equal-weight RRF —
// the wrong document would come first. That fusion cost 11 of 48 measured
// queries their Top-1, which is what this guards against.
func TestFTSWidensCandidatesWithoutTakingOverRanking(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-ranking")
	markCatalog(t, pool, curator.workspaceID)
	// "borogove" is the query term. Only the lexical document contains it, so
	// the FTS leg ranks that one first and returns nothing else.
	lexical := seedSkill(t, pool, curator.workspaceID, "borogove ledger reconciler")
	semantic := seedSkill(t, pool, curator.workspaceID, "mome rath invoice matcher")
	seedBlendedEmbedding(t, pool, lexical, 21, 900, 0.4) // in range, but further
	seedEmbedding(t, pool, semantic, 21)                 // exactly on the query axis

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
	// The FTS-only match is still present — candidate expansion is the whole
	// reason the lexical leg is still wired in.
	if ids[1] != lexical {
		t.Fatalf("FTS candidate expansion dropped its own hit: %v", ids)
	}
	// rank is now cosine similarity, so it has to fall along with the ordering.
	if body.Results[0].Rank == nil || body.Results[1].Rank == nil {
		t.Fatalf("a fully ranked page reported a null rank: %+v", body.Results)
	}
	if *body.Results[0].Rank <= *body.Results[1].Rank {
		t.Fatalf("rank does not follow the ordering: %+v", body.Results)
	}
	// DISC-002 promises 0..1; the vector leg is the only path that can keep it.
	for _, r := range body.Results {
		if *r.Rank < 0 || *r.Rank > 1 {
			t.Fatalf("rank %v is outside the documented 0..1", *r.Rank)
		}
	}
}

// DISC-005 / golden-query-set.md §4.3: an off-topic query gets an explicit
// no-results state, not the catalogue's nearest-but-irrelevant document. Before
// the cut-off, every off-topic query in that measurement returned a confident
// wrong answer — coffee grinding matched a pitch-deck skill.
func TestOffTopicQueryIsRefusedWithASuggestion(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-threshold")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "jubjub ledger reconciler")
	seedEmbedding(t, pool, published, 33)

	// The query embeds onto an unrelated axis: cosine distance 1.0, well past
	// the 0.75 cut-off, and sharing no token with the document either.
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
	// It is a real answer, not a broken one: the vector leg ran fine.
	if body.Degraded {
		t.Fatalf("a refusal was reported as a degradation: %q", body.DegradedReason)
	}
}

// The other side of the cut-off: a genuine match sits comfortably inside it.
// A threshold that refuses real queries is worse than no threshold, and
// golden-query-set.md §10.5 chose 0.75 precisely for 0% recall loss.
func TestRelevantQueryIsNotRefusedByTheCutOff(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-threshold-ok")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "slithy ledger reconciler")
	// 0.3 cosine similarity: a weak-but-real match, above the 0.25 floor. The
	// seed stays at 0.3 after the v2 re-derivation raised that floor, because
	// 0.3 is now what a genuinely weak answer actually scores — golden-query-set
	// §10.5 measured the worst real answer in the corpus at 0.290. The margin is
	// deliberately thin: this test is the boundary, so it should fail if the
	// cut-off ever climbs past what real answers score.
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

// ADR-013 定案調整 1/2: the vector leg carries cross-language recall, so losing
// it is a degradation to be declared, not a silent quality drop. Losing it must
// not lose the answer either — FTS-only is the availability floor.
func TestSearchDegradesToFTSWhenEmbeddingFails(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-degrade")
	markCatalog(t, pool, curator.workspaceID)
	published := seedSkill(t, pool, curator.workspaceID, "quixotical ledger reconciler")
	seedEmbedding(t, pool, published, 3)

	// embedAxis < 0 and no reason text: the whole LLM service is unreachable,
	// which is the realistic shape of this outage.
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
	// The reason must still be honest about where it came from: the LLM leg is
	// not reachable in this state either.
	if got := body.Results[0].MatchReasonSource; got != "template" {
		t.Fatalf("match_reason_source = %q, want template", got)
	}
	// The degraded page is ordered by ts_rank_cd, which is unbounded and is not
	// a cosine similarity. It used to be returned in `rank` regardless, so a
	// field the contract documents as 0..1 came back as 1.4. Null plus a note is
	// the answer; a rescaled lexical score would be a similarity nobody computed.
	for _, r := range body.Results {
		if r.Rank != nil {
			t.Fatalf("degraded answer reported a similarity it never computed: %v", *r.Rank)
		}
		if r.RankNote == "" {
			t.Fatal("null rank with no explanation of what ordered the page")
		}
	}
}

// DISC-002: every result row carries the seven columns a user chooses between
// candidates on — name, plain summary, source tier, agent compatibility,
// dependencies, a risk hint and the last verification time. The four that are
// not free are checked here against a real import, because three of them are
// derived (tier, spec_validation, verified_at) and one is projected at import
// time (risk), and each derivation has a different way of going wrong.
func TestSearchResultsCarryTheDISC002Columns(t *testing.T) {
	pool := requireDB(t)

	a := newAPI(t, pool)
	curator := a.login(t, "curator-facets")
	markCatalog(t, pool, curator.workspaceID)

	// A real import, not a seeded row: the risk facet is projected by the scan
	// the import ran, and the seed helpers never run one.
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
	// 來源層級: indexed, never curated — nothing records a human review yet, and
	// catalog membership is not one (PDM-002).
	if got.Tier.Value != "indexed" || got.Tier.Label == "" {
		t.Fatalf("tier = %+v, want indexed with its copy", got.Tier)
	}
	// 風險提示: the package ships a script, so the scan has something to say and
	// the row must say it without re-reading the package.
	if got.Risk.ScanStatus != "scanned" {
		t.Fatalf("risk scan_status = %q, want scanned", got.Risk.ScanStatus)
	}
	if !hasDisclosureCode(got.Risk.Disclosures, "script-file") {
		t.Fatalf("a package containing a script disclosed none: %+v", got.Risk.Disclosures)
	}
	if got.Risk.Level == "none" {
		t.Fatalf("risk level = none for a package with disclosures: %+v", got.Risk)
	}
	// 相容狀態: spec passed (the import would have been blocked otherwise), the
	// two sandbox axes explicitly unverified — DISC-002's 尚未試跑.
	if got.Compatibility.SpecValidation.Value != "passed" {
		t.Fatalf("spec_validation = %q for an accepted import", got.Compatibility.SpecValidation.Value)
	}
	if got.Compatibility.Capability.Value != "unverified" || got.Compatibility.Runtime.Value != "unverified" {
		t.Fatalf("sandbox axes claimed a verdict before M2: %+v", got.Compatibility)
	}
	// 最近驗證時間: the version's creation time, which is the import that scanned it.
	if got.VerifiedAt == "" {
		t.Fatal("no verification time on a result with a saved version")
	}
	// 依賴 is always present as a list, empty when enrichment extracted none —
	// absent would read as "no dependencies" rather than "not extracted".
	if got.Dependencies == nil {
		t.Fatal("dependencies omitted; an absent list reads as 'none'")
	}
}

// A skill with no saved version is a real state (a fork created ahead of its
// content). Nothing was ever validated for it, so it must not inherit the
// "passed" that being listed implies for everything else.
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
	// Seeded rows predate the scan projection, which is exactly the shape a row
	// written by the SQL-only reindex phase has: unknown, never clean.
	if got.Risk.ScanStatus != "unavailable" {
		t.Fatalf("risk scan_status = %q for a row with no scan", got.Risk.ScanStatus)
	}
}

// No LLM service configured at all is the other half of the same floor.
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

// import-report.md §6.1 bug 2: a document whose enrichment has not landed has no
// embedding, so the ranking leg cannot see it and the cut-off cannot judge it.
// That is index coverage, and it gets its own flag — folding it into `degraded`
// would make a normal catalogue state indistinguishable from an embed outage.
func TestPartialIndexIsReportedSeparatelyFromDegradation(t *testing.T) {
	pool := requireDB(t)

	curator := newAPI(t, pool).login(t, "curator-partial")
	markCatalog(t, pool, curator.workspaceID)
	enriched := seedSkill(t, pool, curator.workspaceID, "mimsy ledger reconciler")
	seedEmbedding(t, pool, enriched, 55)
	// Left as seedSkill created it: enrichment_status 'pending', embedding NULL.
	// It can only reach the page through the lexical leg.
	pending := seedSkill(t, pool, curator.workspaceID, "mimsy invoice matcher")

	a := newAPIWithLLM(t, pool, stubLLM(t, 55, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=mimsy")
	if body.Degraded {
		t.Fatalf("index coverage was reported as an outage: %q", body.DegradedReason)
	}
	if !contains(body.ids(), pending) {
		t.Fatalf("a pending document was hidden from search entirely: %v", body.ids())
	}
	if !body.PartialIndex {
		t.Fatal("a page containing a not-yet-enriched document did not report partial_index")
	}

	// Control: the same query against a fully enriched page says so.
	seedEmbedding(t, pool, pending, 55)
	if body := anon.search(t, "/api/skills/search?q=mimsy"); body.PartialIndex {
		t.Fatalf("fully ranked page reported partial_index: %v", body.ids())
	}
}

// --- DISC-002: match reasons ------------------------------------------------

// DISC-002: every hit carries a reason, and a model-written one is labelled as
// model-written (ADR-013).
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

	// Same corpus, but the reason endpoint is down: the answer still explains
	// itself, now labelled as a template.
	tmpl := newAPIWithLLM(t, pool, stubLLM(t, 11, ""))
	anon = &client{Client: http.DefaultClient, base: tmpl.URL}
	body = anon.search(t, "/api/skills/search?q=frumious")
	if len(body.Results) == 0 {
		t.Fatal("no results to explain on the template path")
	}
	if got := body.Results[0].MatchReasonSource; got != "template" {
		t.Fatalf("match_reason_source = %q, want template", got)
	}
	// The template is the platform's own sentence, so the platform owns its
	// language too — unlike the model-written reason above, which is whatever
	// the LLM returned.
	requireInterfaceLanguage(t, "the template match reason", body.Results[0].MatchReason)
}

// DISC-001: a blank or unusable query is not a search, and it must not become
// one just because the vector leg would happily return the whole catalog.
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
		// DISC-001 asks for a prompt to add the task, input and expected output —
		// an unexplained empty list is not that.
		if !body.NoResults || body.QuerySuggestion == "" {
			t.Errorf("q=%q returned an empty list with no explanation", q)
		}
	}

	// And no q at all is a different answer from a blank one. public.yaml marks
	// q required; a 200 here made the handler looser than the contract every
	// generated client is built from, and the difference between these two
	// requests is the whole of this assertion.
	resp, err := anon.Get(a.URL + "/api/skills/search")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a search with no q parameter: got %d, want 400", resp.StatusCode)
	}
}

// --- DISC-003: structured filters -------------------------------------------

// Both live dimensions, each on its own and then together, on the degraded
// FTS-only path (no LLM service configured).
//
// The degraded path is the one under test on purpose. It is the availability
// floor, the filter columns are projection columns that owe nothing to the
// embedding leg, and an answer that quietly stopped honouring the user's
// filters because a model was down would be a page that lies about what it
// contains — worse than the reduced recall the degradation already declares.
func TestFiltersNarrowOnRealEvidenceIncludingTheDegradedPath(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filters")
	markCatalog(t, pool, curator.workspaceID)

	// Real imports: the scan facts the filter reads are projected by the import,
	// and the seed helpers never run a scan.
	scripted := importPackage(t, pool, a.packages, curator, "uffish-scripted-reporter", true)
	plain := importPackage(t, pool, a.packages, curator, "uffish-plain-reporter", false)
	// Never imported, so no scan was ever projected and no version was ever
	// saved. It is the row both filters have to refuse to guess about.
	unscanned := seedSkill(t, pool, curator.workspaceID, "uffish unscanned reporter")

	anon := &client{Client: http.DefaultClient, base: a.URL}

	// Unfiltered control: all three are reachable, so anything missing below was
	// removed by a filter and not by the query.
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
		// The unscanned row is excluded from `no` as well: nothing scanned it, so
		// "has no script" is not a fact anyone established about it. Answering
		// otherwise is the 不得自行推定為通過 that 02:DISC-004 forbids.
		{"script=no", "&script=no", []string{plain}, []string{scripted, unscanned}},
		{"validation=passed", "&validation=passed", []string{scripted, plain}, []string{unscanned}},
		{"validation=unverified", "&validation=unverified", []string{unscanned}, []string{scripted, plain}},
		// Combined: the two dimensions intersect rather than replacing each other.
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
			// A filtered page is still a filtered page, not a refusal.
			if body.NoResults || body.FilteredOut {
				t.Errorf("a page with results reported an empty state: %+v", body)
			}
		})
	}
}

// The same filters on the hybrid path, where ranking is real. Filtering must
// remove rows and change nothing else: the survivors keep the order and the
// similarity scores they had before the filter was applied.
func TestFiltersOnTheHybridPathRemoveRowsWithoutReranking(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filters-hybrid")
	markCatalog(t, pool, curator.workspaceID)

	scripted := importPackage(t, pool, a.packages, curator, "outgrabe-scripted-reporter", true)
	plain := importPackage(t, pool, a.packages, curator, "outgrabe-plain-reporter", false)
	// The scripted one is the closer match, so it leads the unfiltered page and
	// the scriptless one is what a script=no filter has to be able to promote.
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
	// Filtering is not ranking (the UI says so in the ranking explainer): the
	// surviving row keeps the score it was given, rather than being rescored
	// against a smaller field.
	if got := filtered.Results[0].Rank; got == nil || *got != *plainRank {
		t.Fatalf("rank changed under filtering: %v, want %v", got, *plainRank)
	}
}

// The two empty pages a user can land on are different problems with opposite
// fixes, so the response has to tell them apart. Getting this wrong tells
// someone to reword a query that matched perfectly well.
func TestFilteredToEmptyIsNotTheNoResultsRefusal(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filtered-empty")
	markCatalog(t, pool, curator.workspaceID)
	importPackage(t, pool, a.packages, curator, "galumph-scripted-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	// The query matched; the filter removed the only match.
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
	// The refine-your-query copy belongs to the other state. Offering it here
	// would send the user to rewrite a query that was never the problem.
	if filtered.QuerySuggestion != "" {
		t.Fatalf("filtered-out answer carried the query suggestion: %q", filtered.QuerySuggestion)
	}

	// Same filter, a query nothing matches: now it really is no_results, and
	// filtered_out must not be claimed — there was nothing for a filter to remove.
	refused := anon.search(t, "/api/skills/search?q=whiffling&script=no")
	if !refused.NoResults {
		t.Fatalf("an unmatched query was not refused: %+v", refused)
	}
	requireInterfaceLanguage(t, "the query suggestion on the unmatched-query refusal", refused.QuerySuggestion)
	if refused.FilteredOut {
		t.Fatal("a query that matched nothing blamed the filters for the empty page")
	}
}

// 02:DISC-002 names six filter dimensions and this build has per-row data for
// two. The other four are rejected rather than ignored: a shared or hand-edited
// URL asking for one must not come back as the whole catalog looking like a
// filtered subset. The UI shows the remaining ones as disabled controls with the
// reason, so nothing about them is hidden — only refused.
//
// `tier` left this list on 2026-08-28: migration 0042 gave the catalogue a
// second value, so the dimension can now separate something. What replaces it
// here is `tier=external`, which is a different refusal — the dimension is live
// but that value is not a row anything can hold, exactly like `agent=claude`.
func TestFilterDimensionsWithoutDataAreRejectedNotIgnored(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filter-reject")
	markCatalog(t, pool, curator.workspaceID)
	importPackage(t, pool, a.packages, curator, "beamish-scripted-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, q := range []string{
		"&category=documents", // curation-only, never persisted
		"&agent=claude",       // the dimension is live (0022), this value is not
		"&tier=external",      // the dimension is live (0042), this value is not
		//                        a row: an external result was never imported
		"&mcp=no",            // no signal exists anywhere
		"&script=maybe",      // outside the enum
		"&validation=failed", // spec validation is never reported as failed
		"&script=",           // present-but-empty is outside the enum too
		"&validation=",
		"&agent=",
		"&tier=",
	} {
		if got := anon.status(t, http.MethodGet, "/api/skills/search?q=beamish"+q); got != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 — an unusable filter must not be silently dropped", q, got)
		}
		if got := anon.status(t, http.MethodGet, "/api/skills/catalog?"+strings.TrimPrefix(q, "&")); got != http.StatusBadRequest {
			t.Errorf("catalog %s: got %d, want 400 — both handwritten routes must enforce the contract", q, got)
		}
	}

	// A filter this build does support is still honoured, so the rejection above
	// is about the dimension and not about filtering in general.
	if body := anon.search(t, "/api/skills/search?q=beamish&script=yes"); len(body.Results) != 1 {
		t.Fatalf("a supported filter was rejected too: %+v", body)
	}
}

// restrict puts a licensing hold on a skill the way the bulk script does (0023,
// tools/content/restrict-anthropic-sa-display.sql). Direct SQL on purpose: these
// tests are about what a hold *does* to the read and run paths, and going
// through the operator endpoint (02:SEC-011, PUT /admin/skills/{id}/restriction,
// covered in operator_integration_test.go) would make them fail for reasons that
// have nothing to do with the hold.
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

// The 方案 C shape (m2/anthropic-sa-license-memo.md, owner's decision 2026-08-16):
// a licensing hold closes the paths that hand over the package's own bytes and
// leaves everything the platform wrote about it open. Both halves are asserted,
// because either one alone is a different product: closing the listing too would
// be方案 A, and closing nothing would be 方案 B.
func TestLicensingHoldClosesTheMaterialsAndKeepsTheListing(t *testing.T) {
	pool := requireDB(t)

	a := newAPI(t, pool)
	curator := a.login(t, "curator-hold")
	markCatalog(t, pool, curator.workspaceID)

	held := importPackage(t, pool, a.packages, curator, "brillig-restricted-writer", true)
	free := importPackage(t, pool, a.packages, curator, "brillig-open-writer", true)
	restrict(t, pool, held)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	// Still listed and still ranked: the hold is not a takedown.
	body := anon.search(t, "/api/skills/search?q=brillig")
	names := map[string]bool{}
	for _, r := range body.Results {
		names[r.Name] = true
	}
	if !names["brillig-restricted-writer"] || !names["brillig-open-writer"] {
		t.Fatalf("search dropped a skill it should still list: %+v", body.Results)
	}

	// Detail answers, says why, and still carries the platform's own description.
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

	// The one endpoint that reproduces the package verbatim is closed — with the
	// reason, and as 403 rather than 404, because search just listed it.
	code, files := anon.doJSON(t, http.MethodGet, "/api/skills/"+held+"/files", "")
	if code != http.StatusForbidden {
		t.Fatalf("GET /files on a held skill answered %d, want 403", code)
	}
	if msg, _ := files["error"].(string); msg == "" {
		t.Fatal("the refusal carried no reason; a bare 403 is indistinguishable from a bug")
	}

	// Nothing else moved.
	code, open := anon.doJSON(t, http.MethodGet, "/api/skills/"+free, "")
	if code != http.StatusOK || open["access_restriction"] != nil {
		t.Fatalf("the hold reached a skill it was not applied to: code=%d %+v", code, open["access_restriction"])
	}
	if code := anon.status(t, http.MethodGet, "/api/skills/"+free+"/files"); code != http.StatusOK {
		t.Fatalf("GET /files on an unrestricted skill answered %d, want 200", code)
	}
}

// The third enforcement point of the same decision: a held skill is not copied
// into a sandbox either (0023, gate B). It lives beside the two above rather
// than with the other run tests because the hold is one decision with three
// places that honour it, and the failure mode worth guarding against is somebody
// changing one of the three.
//
// The refusal must arrive before anything else can refuse first — no permission
// confirmation, no provider, no scan verdict is set up here, and the answer is
// still the licensing one.
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

// hasDisclosureCode reports whether the served list carries one catalogue code.
// Codes, not labels: the wording is the server's and may be edited, the identity
// may not (04 丙-29 ④).
func hasDisclosureCode(list []struct{ Code, Label, Note string }, code string) bool {
	for _, d := range list {
		if d.Code == code {
			return true
		}
	}
	return false
}

// 設計系統 §4.3: 「任何被截斷的清單都必須說出總數與截斷理由」 — 「共 N 筆，這裡顯示
// M 筆，因為 X」. The reason half shipped with the truncation notice; the count
// half could only manage 「超過 N 個」, a lower bound from which a reader cannot
// tell 21 from 2100.
//
// The assertion that matters is the RELATIONSHIP, not the number. A total is only
// worth printing if it describes the list printed under it, and the way that
// breaks is drift: the count and the rows stop being produced by the same
// predicates. So this pins both ends — an untruncated page must have total
// exactly equal to its own length, and a truncated one must have strictly more —
// which is precisely what a second COUNT query would eventually fail.
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
	// The point of the field: the cut page reports the same population the full
	// page did, rather than a bound derived from its own length.
	if cut.Total != full.Total {
		t.Errorf("the total describes the matches, not the page: cut=%d, full=%d",
			cut.Total, full.Total)
	}
	if cut.Total <= len(cut.Results) {
		t.Errorf("a truncated page reporting total=%d for %d rows says nothing was cut",
			cut.Total, len(cut.Results))
	}
}

// `limit` is the schema both search endpoints declare and, until 2026-08-25, the
// one violation of it they swallowed: `limit=0`, `limit=500` and `limit=abc` all
// fell back to 20 in the same handler that answers 400 to an over-long `q` and to
// an unusable filter value. One handler, two answers to "this violates the
// schema".
//
// The cost was not pedantry. A caller who asked for 500 to page through the
// catalogue got 20 rows and truncated=true, and read that as "the catalogue holds
// a bit over 20" — ADR-042 決策 3 wants a truncation to declare itself, and what
// declared itself there was a ceiling nobody asked for.
//
// Both endpoints, because both parse the same parameter from the same helper: a
// fix on the public one alone would leave the private one answering the old way.
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
		// minimum: 1, maximum: 100 are inclusive, and no `limit` at all is the
		// declared default rather than an error.
		for _, ok := range []string{"", "&limit=1", "&limit=100", "&limit=20"} {
			if got := ep.cl.status(t, http.MethodGet, ep.path+ok); got != http.StatusOK {
				t.Errorf("%s search%q: got %d, want 200", ep.name, ok, got)
			}
		}
		// Below the minimum, above the maximum, not a number, and present but
		// empty — `allowEmptyValue` is not set, so the last one is not an integer
		// the schema describes either.
		for _, bad := range []string{"&limit=0", "&limit=-1", "&limit=101", "&limit=500", "&limit=abc", "&limit=1.5", "&limit="} {
			if got := ep.cl.status(t, http.MethodGet, ep.path+bad); got != http.StatusBadRequest {
				t.Errorf("%s search%q: got %d, want 400", ep.name, bad, got)
			}
		}
	}
}
