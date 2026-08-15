// DISC-001 / DISC-002 / WS-001 database-backed tests. Shared harness (TestMain,
// migrate, requireDB, login, seedSkill) lives in authz_integration_test.go.
package identity_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

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

type searchBody struct {
	Query   string `json:"query"`
	Results []struct {
		SkillID           string  `json:"skill_id"`
		Rank              float64 `json:"rank"`
		MatchReason       string  `json:"match_reason"`
		MatchReasonSource string  `json:"match_reason_source"`
	} `json:"results"`
	Degraded        bool   `json:"degraded"`
	DegradedReason  string `json:"degraded_reason"`
	PartialIndex    bool   `json:"partial_index"`
	NoResults       bool   `json:"no_results"`
	QuerySuggestion string `json:"query_suggestion"`
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
	if body.Results[0].Rank <= body.Results[1].Rank {
		t.Fatalf("rank does not follow the ordering: %v", body.Results)
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
