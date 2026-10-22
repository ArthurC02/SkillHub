package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

type embedPass struct {
	seen              []string
	done, failed      int
	rewriteBeforeSave bool
}

func runEmbedPass(t *testing.T, pool *pgxpool.Pool, pass *embedPass) {
	t.Helper()
	backfill := &catalog.EnrichmentBackfill{Pool: pool}
	var err error
	pass.done, pass.failed, err = backfill.EmbedMissing(context.Background(), 10, func(ctx context.Context, doc catalog.MissingEmbedding) *pgvector.Vector {
		pass.seen = append(pass.seen, uuidText(doc.SkillID)+":"+doc.EnrichedSummary)
		if pass.rewriteBeforeSave {
			if _, err := pool.Exec(ctx, `UPDATE search_documents SET updated_at = now() WHERE skill_id = $1`, doc.SkillID); err != nil {
				t.Fatal(err)
			}
		}
		vector := pgvector.NewVector(make([]float32, 1536))
		return &vector
	})
	if err != nil {
		t.Fatal(err)
	}
}

func embeddingStored(t *testing.T, pool *pgxpool.Pool, skillID string) bool {
	t.Helper()
	var stored bool
	if err := pool.QueryRow(context.Background(), `SELECT embedding IS NOT NULL FROM search_documents WHERE skill_id = $1`, mustUUID(t, skillID)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestTheEmbeddingBackfillEmbedsOnlyEnrichedDocumentsWithoutAVectorAndSkipsARewrittenOne(t *testing.T) {
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

	rewritten := &embedPass{rewriteBeforeSave: true}
	runEmbedPass(t, pool, rewritten)
	if len(rewritten.seen) != 1 || rewritten.seen[0] != f.skillID+":stored enrichment" || rewritten.done != 0 {
		t.Fatalf("first pass saw %v and stored %d; want only the enriched fixture, its vector skipped after the rewrite", rewritten.seen, rewritten.done)
	}
	if embeddingStored(t, pool, f.skillID) {
		t.Fatal("a vector computed before the rewrite was stored")
	}

	leased := &embedPass{}
	if runEmbedPass(t, pool, leased); len(leased.seen) != 0 {
		t.Fatalf("a pass inside the claim lease saw %v, want nothing", leased.seen)
	}

	if _, err := pool.Exec(ctx, `UPDATE search_documents SET enrichment_attempted_at = NULL WHERE skill_id = $1`, mustUUID(t, f.skillID)); err != nil {
		t.Fatal(err)
	}
	current := &embedPass{}
	if runEmbedPass(t, pool, current); current.done != 1 || !embeddingStored(t, pool, f.skillID) || embeddingStored(t, pool, pendingSkill) {
		t.Fatalf("pass against the current write stored %d; want the fixture's vector and none for the pending document", current.done)
	}
}
