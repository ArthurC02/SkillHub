// DISC-001 / DISC-002 / WS-001 database-backed tests. Shared harness (TestMain,
// migrate, requireDB, login, seedSkill) lives in authz_integration_test.go.
package identity_test

import (
	"context"
	"encoding/json"
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
		SkillID           string `json:"skill_id"`
		MatchReason       string `json:"match_reason"`
		MatchReasonSource string `json:"match_reason_source"`
	} `json:"results"`
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degraded_reason"`
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
	seedEmbedding(t, pool, far, 900)

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
	}
}
