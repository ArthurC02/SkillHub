package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/pgvector/pgvector-go"
)

func TestCreationHybridRetrievalRunsThePublicRuleWithoutUnrankedRows(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	near := newFixture(t, a, pool, uniqueWorklistLabel("hybrid-near"))
	far := newFixture(t, a, pool, uniqueWorklistLabel("hybrid-far"))
	private := newFixture(t, a, pool, uniqueWorklistLabel("hybrid-private"))
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
		{private, "pii-flag-private", 2, catalog.LexicalIndexText("pii-flag", "flag personal data 標記個資")},
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

	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cost := 0.00001
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.EmbedResponse{Embeddings: [][]float32{unit(0).Slice()}, Model: "stub", Dimensions: 1536, Usage: &llmclient.GatewayUsage{CostUSD: &cost}})
	}))
	t.Cleanup(embed.Close)
	svc := &catalog.Service{Pool: pool, LLM: &llmclient.Client{BaseURL: embed.URL}, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}

	ids, cost, degraded, err := svc.CreationKnowledgeIDs(ctx, "pii-flag 標記個資", catalog.CreationMaxDistance)
	if err != nil || degraded || cost != 0.00001 {
		t.Fatalf("ids=%v cost=%v degraded=%v err=%v", ids, cost, degraded, err)
	}
	if len(ids) != 2 || ids[0] != far.skillID || ids[1] != near.skillID {
		t.Fatalf("the covered lexical hit first, then the vector hit: got %v want [%s %s]", ids, far.skillID, near.skillID)
	}

	ids, _, _, err = svc.CreationKnowledgeIDs(ctx, "pii-flag 不存在的詞", catalog.CreationMaxDistance)
	if err != nil || len(ids) != 1 || ids[0] != near.skillID {
		t.Fatalf("partial coverage must not admit: %v err=%v", ids, err)
	}

	ids, _, _, err = svc.CreationKnowledgeIDs(ctx, "remove duplicate rows", catalog.CreationDuplicateDistance)
	if err != nil || len(ids) != 1 || ids[0] != near.skillID {
		t.Fatalf("duplicate cut-off: %v err=%v", ids, err)
	}

	lexOnly := &catalog.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	ids, cost, degraded, err = lexOnly.CreationKnowledgeIDs(ctx, "pii-flag", catalog.CreationMaxDistance)
	if err != nil || !degraded || cost != 0 || len(ids) != 1 || ids[0] != far.skillID {
		t.Fatalf("degraded answer: ids=%v cost=%v degraded=%v err=%v", ids, cost, degraded, err)
	}
}

func TestPublicSearchKeepsACoveredLexicalHitPastTheCutoffAndPinsTheExactName(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	curator := newAPI(t, pool).login(t, "curator-bigram")
	markCatalog(t, pool, curator.workspaceID)
	near := seedSkill(t, pool, curator.workspaceID, "quorble ledger reconciler")
	far := seedSkill(t, pool, curator.workspaceID, "pii-flagger")
	seedEmbedding(t, pool, near, 733)

	seedEmbedding(t, pool, far, 1234)
	q := gen.New(pool)
	for id, text := range map[string]string{near: "quorble ledger reconciler", far: "pii-flagger 遮罩帳號尾碼"} {
		if err := q.SetSearchDocumentBigram(ctx, gen.SetSearchDocumentBigramParams{SkillID: mustUUID(t, id), BigramText: catalog.LexicalIndexText(text)}); err != nil {
			t.Fatal(err)
		}
	}
	a := newAPIWithLLM(t, pool, stubLLM(t, 733, "because it fits"))
	anon := &client{Client: http.DefaultClient, base: a.URL}

	body := anon.search(t, "/api/skills/search?q=pii-flagger")
	if ids := body.ids(); body.Degraded || body.NoResults || len(ids) != 2 || ids[0] != far || ids[1] != near {
		t.Fatalf("exact name must be first and kept past the cut-off: %v degraded=%v no_results=%v", ids, body.Degraded, body.NoResults)
	}

	body = anon.search(t, "/api/skills/search?q=%E5%B8%B3%E8%99%9F%E5%B0%BE%E7%A2%BC")
	if ids := body.ids(); len(ids) != 2 || ids[0] != far || ids[1] != near {
		t.Fatalf("a covered term is admitted ahead of the vector hit: %v", ids)
	}

	body = anon.search(t, "/api/skills/search?q=pii-flagger+%E4%B8%8D%E5%AD%98%E5%9C%A8")
	if ids := body.ids(); len(ids) != 1 || ids[0] != near {
		t.Fatalf("partial coverage must not bypass the cut-off: %v", ids)
	}
}

func TestCatalogReferenceFactsReadTheTierAndTheScan(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	curator := newAPI(t, pool).login(t, "curator-facts")
	markCatalog(t, pool, curator.workspaceID)
	skill := seedSkill(t, pool, curator.workspaceID, "facts-holder")
	version := seedSkillVersion(t, pool, curator.workspaceID, skill)
	if _, err := pool.Exec(ctx, `UPDATE search_documents SET scan = '{"warnings": 2, "codes": ["script-file"]}'::jsonb WHERE skill_id = $1`, mustUUID(t, skill)); err != nil {
		t.Fatal(err)
	}
	svc := &catalog.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	tier, scan, warnings, err := svc.CatalogReferenceFacts(ctx, skill, version)
	if err != nil || tier != "indexed" || scan != "scanned" || warnings != 2 {
		t.Fatalf("indexed: tier=%q scan=%q warnings=%d err=%v", tier, scan, warnings, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE skills SET curation_tier = 'curated', curated_version_id = $2 WHERE id = $1`, mustUUID(t, skill), mustUUID(t, version)); err != nil {
		t.Fatal(err)
	}
	if tier, _, _, err = svc.CatalogReferenceFacts(ctx, skill, version); err != nil || tier != "curated" {
		t.Fatalf("curated version: tier=%q err=%v", tier, err)
	}

	other := uuidText(creationID(t))
	if tier, _, _, err = svc.CatalogReferenceFacts(ctx, skill, other); err != nil || tier != "indexed" {
		t.Fatalf("other version: tier=%q err=%v", tier, err)
	}

	private := newFixture(t, newAPI(t, pool), pool, uniqueWorklistLabel("facts-private"))
	if tier, scan, _, err = svc.CatalogReferenceFacts(ctx, private.skillID, private.versionID); err == nil || tier != "unknown" || scan != "unknown" {
		t.Fatalf("private skill: tier=%q scan=%q err=%v", tier, scan, err)
	}
}

func TestResetCatalogueEnrichmentBeforeQueuesOnlyOlderPromptVersions(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	curator := newAPI(t, pool).login(t, "curator-reenrich")
	markCatalog(t, pool, curator.workspaceID)
	old := seedSkill(t, pool, curator.workspaceID, "reenrich-old")
	current := seedSkill(t, pool, curator.workspaceID, "reenrich-current")
	private := newFixture(t, newAPI(t, pool), pool, uniqueWorklistLabel("reenrich-private"))
	q := gen.New(pool)
	set := func(skill, version string) {
		if _, err := pool.Exec(ctx, "UPDATE search_documents SET enrichment_status = 'enriched', enrichment_prompt_version = $2 WHERE skill_id = $1", mustUUID(t, skill), version); err != nil {
			t.Fatal(err)
		}
	}
	set(old, "enrich-skill/v2")
	set(current, "enrich-skill/v7")
	set(private.skillID, "enrich-skill/v2")

	catalogs, err := (&identity.Service{Pool: pool}).CatalogWorkspaceIDs(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	n, err := q.ResetCatalogueEnrichmentBefore(ctx, gen.ResetCatalogueEnrichmentBeforeParams{
		PromptVersion: "enrich-skill/v7", CatalogWorkspaceIds: catalogs,
	})
	if err != nil || n < 1 {
		t.Fatalf("reset %d err=%v, want at least the old catalogue document", n, err)
	}
	status := func(skill string) string {
		var st string
		if err := pool.QueryRow(ctx, "SELECT enrichment_status FROM search_documents WHERE skill_id = $1", mustUUID(t, skill)).Scan(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	if status(old) != "pending" || status(current) != "enriched" || status(private.skillID) != "enriched" {
		t.Fatalf("old=%s current=%s private=%s", status(old), status(current), status(private.skillID))
	}
}

func TestCatalogSkillRisksAnswerOnlyForCatalogSkills(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	curator := a.login(t, "curator-risks")
	markCatalog(t, pool, curator.workspaceID)
	public := seedSkill(t, pool, curator.workspaceID, "risks-public")
	private := newFixture(t, a, pool, uniqueWorklistLabel("risks-private"))
	for _, id := range []string{public, private.skillID} {
		tag, err := pool.Exec(ctx, `UPDATE search_documents SET scan = '{"warnings": 3, "codes": ["script-file"]}'::jsonb WHERE skill_id = $1`, mustUUID(t, id))
		if err != nil {
			t.Fatal(err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("skill %s has no search document to carry a scan", id)
		}
	}

	svc := &catalog.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	risks, err := svc.CatalogSkillRisks(ctx, []pgtype.UUID{mustUUID(t, public), mustUUID(t, private.skillID)})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(risks[public]); !strings.Contains(got, `"scan_status":"scanned"`) {
		t.Errorf("a catalog skill's scan was not reported: %s", got)
	}
	if got := string(risks[private.skillID]); !strings.Contains(got, `"scan_status":"unavailable"`) {
		t.Errorf("a private skill's scan was reported as a catalog fact: %s", got)
	}
}
