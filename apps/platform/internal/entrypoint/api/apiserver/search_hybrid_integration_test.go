package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/pgvector/pgvector-go"
)

// The creation tool's hybrid retrieval, on the real tables: the vector leg
// within CreationMaxDistance in rank order, then one lexical admission that
// carries every token of the query (creation-measure/search-f1, 2026-09-06),
// and the lexical-only answer when no embedding service is wired.
func TestCreationHybridRetrievalAdmitsACoveredLexicalHitAfterTheVectorLeg(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	near := newFixture(t, a, pool, uniqueWorklistLabel("hybrid-near"))
	far := newFixture(t, a, pool, uniqueWorklistLabel("hybrid-far"))
	markCatalog(t, pool, near.workspaceID)
	markCatalog(t, pool, far.workspaceID)
	unit := func(axis int) pgvector.Vector {
		v := make([]float32, 1536)
		v[axis] = 1
		return pgvector.NewVector(v)
	}
	q := gen.New(pool)
	for _, doc := range []struct {
		f      fixture
		name   string
		axis   int
		bigram string
	}{
		{near, "excel-deduplicate", 0, catalog.LexicalIndexText("excel-deduplicate", "remove duplicate rows 去除重複列")},
		{far, "pii-flag", 1, catalog.LexicalIndexText("pii-flag", "flag personal data 標記個資")},
	} {
		emb := unit(doc.axis)
		if err := q.UpsertSearchDocumentEnriched(ctx, gen.UpsertSearchDocumentEnrichedParams{
			SkillID: mustUUID(t, doc.f.skillID), WorkspaceID: mustUUID(t, doc.f.workspaceID),
			Name: doc.name, Summary: doc.name, EnrichedSummary: doc.name, TaskExamples: "[]", Tags: []byte(`[]`),
			Limitations: "[]", Scan: []byte(`{}`), Embedding: &emb, EnrichmentStatus: "enriched", BigramText: doc.bigram,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The embedding service answers every query with axis 0: the near document
	// is at distance 0, the far one at 1 — beyond CreationMaxDistance.
	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cost := 0.00001
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.EmbedResponse{Embeddings: [][]float32{unit(0).Slice()}, Model: "stub", Dimensions: 1536, Usage: &llmclient.GatewayUsage{CostUSD: &cost}})
	}))
	t.Cleanup(embed.Close)
	svc := &catalog.Service{Pool: pool, LLM: &llmclient.Client{BaseURL: embed.URL}}

	ids, cost, degraded, err := svc.CreationKnowledgeIDs(ctx, "pii-flag 標記個資")
	if err != nil || degraded || cost != 0.00001 {
		t.Fatalf("ids=%v cost=%v degraded=%v err=%v", ids, cost, degraded, err)
	}
	if len(ids) != 2 || ids[0] != near.skillID || ids[1] != far.skillID {
		t.Fatalf("vector hit first, then the covered lexical hit: got %v want [%s %s]", ids, near.skillID, far.skillID)
	}
	// A query the far document does not fully cover is not admitted by the
	// lexical leg: the vector leg's answer stands alone.
	ids, _, _, err = svc.CreationKnowledgeIDs(ctx, "pii-flag 不存在的詞")
	if err != nil || len(ids) != 1 || ids[0] != near.skillID {
		t.Fatalf("partial coverage must not admit: %v err=%v", ids, err)
	}
	// No embedding service: the lexical leg alone, flagged degraded.
	lexOnly := &catalog.Service{Pool: pool}
	ids, cost, degraded, err = lexOnly.CreationKnowledgeIDs(ctx, "pii-flag")
	if err != nil || !degraded || cost != 0 || len(ids) != 1 || ids[0] != far.skillID {
		t.Fatalf("degraded answer: ids=%v cost=%v degraded=%v err=%v", ids, cost, degraded, err)
	}
}

// 05 R-48 (2026-09-06): the public search's third candidate leg is the bigram
// column. A document that carries every token of the query is kept past the
// distance cut-off and ranked before the vector hits — the name or the
// distinctive term a person typed on purpose (search-f1/results-public-rule:
// distinctive terms Top-1 7 → 23 of 25, golden set unchanged) — and the exact
// name is pinned first; a query the document covers only in part is not
// admitted by that leg.
func TestPublicSearchKeepsACoveredLexicalHitPastTheCutoffAndPinsTheExactName(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	curator := newAPI(t, pool).login(t, "curator-bigram")
	markCatalog(t, pool, curator.workspaceID)
	near := seedSkill(t, pool, curator.workspaceID, "quorble ledger reconciler")
	far := seedSkill(t, pool, curator.workspaceID, "pii-flagger")
	seedEmbedding(t, pool, near, 733)
	// Orthogonal to every query the stub embeds: distance 1, past MaxCosineDistance.
	seedEmbedding(t, pool, far, 1234)
	q := gen.New(pool)
	for id, text := range map[string]string{near: "quorble ledger reconciler", far: "pii-flagger 遮罩帳號尾碼"} {
		if err := q.SetSearchDocumentBigram(ctx, gen.SetSearchDocumentBigramParams{SkillID: mustUUID(t, id), BigramText: catalog.LexicalIndexText(text)}); err != nil {
			t.Fatal(err)
		}
	}
	a := newAPIWithLLM(t, pool, stubLLM(t, 733, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	// The exact name: covered, past the cut-off, pinned before the vector hit.
	body := anon.search(t, "/api/skills/search?q=pii-flagger")
	if ids := body.ids(); body.Degraded || body.NoResults || len(ids) != 2 || ids[0] != far || ids[1] != near {
		t.Fatalf("exact name must be first and kept past the cut-off: %v degraded=%v no_results=%v", ids, body.Degraded, body.NoResults)
	}
	// One distinctive Chinese term (帳號尾碼): covered, ranked before the vector hit.
	body = anon.search(t, "/api/skills/search?q=%E5%B8%B3%E8%99%9F%E5%B0%BE%E7%A2%BC")
	if ids := body.ids(); len(ids) != 2 || ids[0] != far || ids[1] != near {
		t.Fatalf("a covered term is admitted ahead of the vector hit: %v", ids)
	}
	// Partial coverage (pii-flagger 不存在): the bigram leg does not admit it.
	body = anon.search(t, "/api/skills/search?q=pii-flagger+%E4%B8%8D%E5%AD%98%E5%9C%A8")
	if ids := body.ids(); len(ids) != 1 || ids[0] != near {
		t.Fatalf("partial coverage must not bypass the cut-off: %v", ids)
	}
}
