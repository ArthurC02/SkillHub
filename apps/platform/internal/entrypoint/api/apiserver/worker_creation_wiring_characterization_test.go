package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
)

const (
	knowledgeAxis     = 1473
	knowledgeCallCost = 0.00001
)

func knowledgeWorkers(t *testing.T, a *api, pool *pgxpool.Pool, word string, axis int) *creation.Service {
	t.Helper()
	ctx := context.Background()
	store, stop, err := objstore.NewInProcess("skillhub")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	for key, data := range a.packages {
		if strings.HasPrefix(key, "packages/hash-"+word) {
			if err := store.Put(ctx, key, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cost := knowledgeCallCost
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.EmbedResponse{
			Embeddings: [][]float32{unitVector(axis)}, Model: "stub", Dimensions: embedDims,
			Usage: &llmclient.GatewayUsage{CostUSD: &cost, CostSource: llmclient.CostSourceGateway},
		})
	}))
	t.Cleanup(embed.Close)
	set, err := worker.BuildWorkers(pool, worker.Deps{Store: store, LLM: &llmclient.Client{BaseURL: embed.URL, Token: "test-service"}})
	if err != nil {
		t.Fatal(err)
	}
	return set.Creation
}

func TestWorkerKnowledgeSearchFusesEveryQueryAndResolvesAtMostThree(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("knowledgecap")
	shelf := shelveReferences(t, a, pool, word, knowledgeAxis, 4, 1)
	svc := knowledgeWorkers(t, a, pool, word, knowledgeAxis)

	refs, cost, err := svc.SearchKnowledge(context.Background(), shelf.ws, []string{word, word + " again"})
	if err != nil {
		t.Fatal(err)
	}
	if cost != 2*knowledgeCallCost {
		t.Errorf("cost = %v, want the two embedding calls' %v", cost, 2*knowledgeCallCost)
	}
	if len(refs) != creation.MaxReferences {
		t.Fatalf("got %d references %v, want %d", len(refs), referenceIDs(refs), creation.MaxReferences)
	}
	for _, r := range refs {
		if !r.Available || !contains(shelf.skills[:4], r.SkillID) {
			t.Errorf("reference %+v is not one of the shelf's readable skills", r)
		}
	}
}

func TestWorkerKnowledgeSearchLeavesOutWhatCannotBeRead(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("knowledgeskip")
	shelf := shelveReferences(t, a, pool, word, knowledgeAxis+1, 1, 1)
	svc := knowledgeWorkers(t, a, pool, word, knowledgeAxis+1)

	refs, _, err := svc.SearchKnowledge(context.Background(), shelf.ws, []string{word})
	if err != nil {
		t.Fatal(err)
	}
	if got := referenceIDs(refs); len(got) != 1 || got[0] != shelf.skills[0] {
		t.Errorf("got %v, want only the readable skill %s", got, shelf.skills[0])
	}
}

func TestWorkerKnowledgeSearchThatCannotRunReportsWhy(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("knowledgeerr")
	shelf := shelveReferences(t, a, pool, word, knowledgeAxis+2, 1, 0)
	svc := knowledgeWorkers(t, a, pool, word, knowledgeAxis+2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	refs, _, err := svc.SearchKnowledge(ctx, shelf.ws, []string{word})
	if err == nil || refs != nil {
		t.Errorf("cancelled search: refs=%v err=%v, want no references and the error", referenceIDs(refs), err)
	}
}

func TestWorkerReferenceOfAScannedCatalogSkillCarriesItsFacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("knowledgeref")
	shelf := shelveReferences(t, a, pool, word, knowledgeAxis+3, 1, 1)
	setScan(t, pool, shelf.skills[0], `{"warnings": 1, "codes": []}`)
	svc := knowledgeWorkers(t, a, pool, word, knowledgeAxis+3)

	ref, content, err := svc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], shelf.versions[0])
	if err != nil {
		t.Fatal(err)
	}
	if !ref.Available || ref.VersionID != shelf.versions[0] || ref.Tier != "indexed" || ref.ScanStatus != "scanned" || ref.Warnings == nil || *ref.Warnings != 1 {
		t.Errorf("reference = %+v, want available at the stated version with indexed/scanned/1", ref)
	}
	if !strings.Contains(content.SkillMD, "Just prose.") {
		t.Errorf("content = %+v, want the package's SKILL.md", content)
	}

	setScan(t, pool, shelf.skills[0], `[]`)
	if ref, _, err = svc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], ""); err != nil || ref.ScanStatus != "unavailable" || ref.Warnings != nil {
		t.Errorf("unreadable scan: %+v err=%v, want unavailable and no warning count", ref, err)
	}
	if ref, _, err = svc.ResolveReference(context.Background(), shelf.ws, shelf.skills[1], ""); err == nil || ref.Available {
		t.Errorf("missing package: available=%v err=%v, want unavailable with an error", ref.Available, err)
	}
	assertMalformedReferenceIDsRefused(t, svc, shelf)
}

func assertMalformedReferenceIDsRefused(t *testing.T, svc *creation.Service, shelf referenceShelf) {
	t.Helper()
	for _, ids := range [][2]string{{"not-a-uuid", ""}, {shelf.skills[0], "not-a-uuid"}} {
		if ref, _, err := svc.ResolveReference(context.Background(), shelf.ws, ids[0], ids[1]); !errors.Is(err, creation.ErrInvalidCommand) || ref != (creation.Reference{}) {
			t.Errorf("malformed %v: ref=%+v err=%v, want the zero reference and ErrInvalidCommand", ids, ref, err)
		}
	}
}
