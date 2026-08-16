// DISC-001 / DISC-002 / WS-001 database-backed tests. Shared harness (TestMain,
// migrate, requireDB, login, seedSkill) lives in authz_integration_test.go.
package identity_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

const embedDims = 1536

// --- helpers ---------------------------------------------------------------

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
	svc := &ingest.Service{Pool: pool, Store: store}
	res, err := svc.UploadZip(ctx, ws, namedPackage(t, name, withScript))
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
		ScanStatus      string `json:"scan_status"`
		Level           string `json:"level"`
		Warnings        int    `json:"warnings"`
		HasScripts      bool   `json:"has_scripts"`
		HasExternalURLs bool   `json:"has_external_urls"`
	} `json:"risk"`
	Dependencies  []string `json:"dependencies"`
	Compatibility struct {
		SpecValidation string `json:"spec_validation"`
		Capability     string `json:"capability"`
		Runtime        string `json:"runtime"`
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
	if body.QuerySuggestion == "" {
		t.Fatal("no-results answer carries no suggestion to refine the query")
	}
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
	if !got.Risk.HasScripts {
		t.Fatal("a package containing a script reported no script in its risk hint")
	}
	if got.Risk.Level == "none" {
		t.Fatalf("risk level = none for a package with disclosures: %+v", got.Risk)
	}
	// 相容狀態: spec passed (the import would have been blocked otherwise), the
	// two sandbox axes explicitly unverified — DISC-002's 尚未試跑.
	if got.Compatibility.SpecValidation != "passed" {
		t.Fatalf("spec_validation = %q for an accepted import", got.Compatibility.SpecValidation)
	}
	if got.Compatibility.Capability != "unverified" || got.Compatibility.Runtime != "unverified" {
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
	if got.Compatibility.SpecValidation != "unverified" {
		t.Fatalf("spec_validation = %q for a skill with nothing saved", got.Compatibility.SpecValidation)
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
	if body.Results[0].MatchReason == "" {
		t.Fatal("template fallback produced no reason")
	}
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
	if !refused.NoResults || refused.QuerySuggestion == "" {
		t.Fatalf("an unmatched query was not refused with a suggestion: %+v", refused)
	}
	if refused.FilteredOut {
		t.Fatal("a query that matched nothing blamed the filters for the empty page")
	}
}

// 02:DISC-002 names six filter dimensions and this build has per-row data for
// two. The other four are rejected rather than ignored: a shared or hand-edited
// URL asking for one must not come back as the whole catalog looking like a
// filtered subset. The UI shows the same four as disabled controls with the
// reason, so nothing about them is hidden — only refused.
func TestFilterDimensionsWithoutDataAreRejectedNotIgnored(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-filter-reject")
	markCatalog(t, pool, curator.workspaceID)
	importPackage(t, pool, a.packages, curator, "beamish-scripted-reporter", true)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	for _, q := range []string{
		"&category=documents", // curation-only, never persisted
		"&tier=curated",       // one value across the whole catalog
		"&agent=claude",       // the dimension is live (0022), this value is not
		"&mcp=no",             // no signal exists anywhere
		"&script=maybe",       // outside the enum
		"&validation=failed",  // spec validation is never reported as failed
	} {
		if got := anon.status(t, http.MethodGet, "/api/skills/search?q=beamish"+q); got != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 — an unusable filter must not be silently dropped", q, got)
		}
	}

	// A filter this build does support is still honoured, so the rejection above
	// is about the dimension and not about filtering in general.
	if body := anon.search(t, "/api/skills/search?q=beamish&script=yes"); len(body.Results) != 1 {
		t.Fatalf("a supported filter was rejected too: %+v", body)
	}
}
