package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestTheEmbeddingBackfillClaimsOnlyEnrichedDocumentsWithoutAVectorAndSkipsARewrittenOne(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	ctx := context.Background()
	tag := uniqueWorklistLabel("missing-embedding")
	f := newFixture(t, a, pool, tag)
	pendingSkill := seedSkill(t, pool, f.workspaceID, tag+"-pending")
	seedVersion(t, pool, f.workspaceID, pendingSkill, tag+"-pending-version")
	if _, err := pool.Exec(ctx, `UPDATE search_documents
		SET enrichment_status = CASE skill_id WHEN $1 THEN 'enriched' ELSE 'pending' END,
		    enriched_summary = 'stored enrichment', embedding = NULL, enrichment_attempted_at = NULL
		WHERE skill_id IN ($1, $2)`, mustUUID(t, f.skillID), mustUUID(t, pendingSkill)); err != nil {
		t.Fatal(err)
	}
	catalogSvc := wiring.NewCatalogService(pool)

	claimed, err := catalogSvc.EnrichedWithoutEmbedding(ctx, 10)
	if err != nil || len(claimed) != 1 || uuidText(claimed[0].SkillID) != f.skillID || claimed[0].EnrichedSummary != "stored enrichment" {
		t.Fatalf("claim = %+v, %v; want only the enriched fixture %s with its stored text", claimed, err, f.skillID)
	}
	if again, err := catalogSvc.EnrichedWithoutEmbedding(ctx, 10); err != nil || len(again) != 0 {
		t.Fatalf("second claim inside the lease = %+v, %v; want nothing", again, err)
	}

	assertTheVectorLandsOnlyOnTheCurrentWrite(t, pool, catalogSvc, claimed[0])
}

func assertTheVectorLandsOnlyOnTheCurrentWrite(t *testing.T, pool *pgxpool.Pool, catalogSvc *catalog.Service, doc catalog.MissingEmbedding) {
	t.Helper()
	ctx := context.Background()
	vector := pgvector.NewVector(make([]float32, 1536))
	if _, err := pool.Exec(ctx, `UPDATE search_documents SET updated_at = now() WHERE skill_id = $1`, doc.SkillID); err != nil {
		t.Fatal(err)
	}
	if stored, err := catalogSvc.StoreMissingEmbedding(ctx, doc.SkillID, doc.WrittenAt, vector); err != nil || stored {
		t.Fatalf("store after a rewrite = %v, %v; want skipped so a stale vector never lands", stored, err)
	}
	writtenAt := doc.WrittenAt
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM search_documents WHERE skill_id = $1`, doc.SkillID).Scan(&writtenAt); err != nil {
		t.Fatal(err)
	}
	if stored, err := catalogSvc.StoreMissingEmbedding(ctx, doc.SkillID, writtenAt, vector); err != nil || !stored {
		t.Fatalf("store against the current write = %v, %v; want stored", stored, err)
	}
	var embedded bool
	if err := pool.QueryRow(ctx, `SELECT embedding IS NOT NULL FROM search_documents WHERE skill_id = $1`, doc.SkillID).Scan(&embedded); err != nil || !embedded {
		t.Fatalf("embedding stored = %v, %v; want true", embedded, err)
	}
}
