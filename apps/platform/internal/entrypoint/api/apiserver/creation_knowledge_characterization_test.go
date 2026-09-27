package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func lexicalCatalogSkills(t *testing.T, pool *pgxpool.Pool, texts ...string) []string {
	t.Helper()
	curator := newAPI(t, pool).login(t, uniqueReferenceWord("lexical"))
	markCatalog(t, pool, curator.workspaceID)
	ids := make([]string, len(texts))
	for i, text := range texts {
		ids[i] = seedSkill(t, pool, curator.workspaceID, text)
		if err := gen.New(pool).SetSearchDocumentBigram(context.Background(), gen.SetSearchDocumentBigramParams{
			SkillID: mustUUID(t, ids[i]), BigramText: catalog.LexicalIndexText(text),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func knowledgeSearch(pool *pgxpool.Pool, llmURL string) *catalog.Service {
	svc := &catalog.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	if llmURL != "" {
		svc.LLM = catalog.ModelOrNone(&llmclient.Client{BaseURL: llmURL})
	}
	return svc
}

func distinct(ids []string) bool {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func TestCreationKnowledgeWithoutAModelFillsFromAnyWordMatchesUpToThree(t *testing.T) {
	pool := requireDB(t)
	w1, w2 := uniqueReferenceWord("lexone"), uniqueReferenceWord("lextwo")
	ids := lexicalCatalogSkills(t, pool, w1+" "+w2, w1+" only", w2+" only", w1+" again")

	got, cost, degraded, err := knowledgeSearch(pool, "").CreationKnowledgeIDs(context.Background(), w1+" "+w2, catalog.CreationMaxDistance)
	if err != nil || !degraded || cost != 0 {
		t.Fatalf("degraded=%v cost=%v err=%v, want a free degraded answer", degraded, cost, err)
	}
	if len(got) != 3 || got[0] != ids[0] || !distinct(got) {
		t.Fatalf("got %v, want the all-words match %s first, then two any-word matches, none repeated", got, ids[0])
	}
	for _, id := range got {
		if !contains(ids, id) {
			t.Errorf("%s is not one of this test's skills %v", id, ids)
		}
	}
}

func TestCreationKnowledgeWithThreeAllWordMatchesLooksNoFurther(t *testing.T) {
	pool := requireDB(t)
	w1, w2 := uniqueReferenceWord("lexthree"), uniqueReferenceWord("lexfull")
	ids := lexicalCatalogSkills(t, pool, w1+" "+w2+" a", w1+" "+w2+" b", w1+" "+w2+" c", w1+" partial")

	got, _, degraded, err := knowledgeSearch(pool, "").CreationKnowledgeIDs(context.Background(), w1+" "+w2, catalog.CreationMaxDistance)
	if err != nil || !degraded {
		t.Fatalf("degraded=%v err=%v", degraded, err)
	}
	if len(got) != 3 || contains(got, ids[3]) || !distinct(got) {
		t.Errorf("got %v, want exactly the three all-words matches %v", got, ids[:3])
	}
}

func TestCreationKnowledgeLeavesOutASkillWaitingForItsEmbedding(t *testing.T) {
	pool := requireDB(t)
	const axis = 1480
	w := uniqueReferenceWord("lexpending")
	ids := lexicalCatalogSkills(t, pool, w+" embedded", w+" pending")
	seedEmbedding(t, pool, ids[0], axis)
	if got, _, _, err := knowledgeSearch(pool, "").CreationKnowledgeIDs(context.Background(), w, catalog.CreationMaxDistance); err != nil || !contains(got, ids[1]) {
		t.Fatalf("the pending skill must be a word match for this test to mean anything: %v err=%v", got, err)
	}

	got, _, degraded, err := knowledgeSearch(pool, stubLLM(t, axis, "")).CreationKnowledgeIDs(context.Background(), w, catalog.CreationMaxDistance)
	if err != nil || degraded || len(got) != 1 || got[0] != ids[0] {
		t.Errorf("ids=%v degraded=%v err=%v, want only the embedded skill %s", got, degraded, err, ids[0])
	}
}

func TestCreationKnowledgeFallsBackToWordsWhenTheEmbeddingFails(t *testing.T) {
	pool := requireDB(t)
	w := uniqueReferenceWord("lexfallback")
	ids := lexicalCatalogSkills(t, pool, w+" skill")
	empty := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		writeJSON(rw, map[string]any{"embeddings": [][]float32{}, "model": "stub", "dimensions": embedDims})
	}))
	t.Cleanup(empty.Close)

	for name, llmURL := range map[string]string{
		"the embedding call errors":        stubLLM(t, -1, ""),
		"the embedding comes back without": empty.URL,
	} {
		got, cost, degraded, err := knowledgeSearch(pool, llmURL).CreationKnowledgeIDs(context.Background(), w, catalog.CreationMaxDistance)
		if err != nil || !degraded || cost != 0 || len(got) != 1 || got[0] != ids[0] {
			t.Errorf("%s: ids=%v cost=%v degraded=%v err=%v, want the word match as a free degraded answer", name, got, cost, degraded, err)
		}
	}
}
