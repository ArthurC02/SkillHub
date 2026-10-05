package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestIndexingASkillKeepsTheEnrichedWordsOfItsDocument(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	owner := newAPI(t, pool).login(t, "owner-keeps-enriched")
	skillID := seedSkill(t, pool, owner.workspaceID, "keeps-enriched")
	if _, err := pool.Exec(ctx, `UPDATE search_documents SET enriched_summary = 'zorblax', task_examples = 'quillfeather', tags = '["snarfwidget"]'::jsonb WHERE skill_id = $1`, mustUUID(t, skillID)); err != nil {
		t.Fatal(err)
	}
	svc := wiring.NewCatalogService(pool)

	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return svc.IndexSkill(ctx, tx, catalog.SkillProjection{
			SkillID: mustUUID(t, skillID), WorkspaceID: mustUUID(t, owner.workspaceID), Name: "keeps-enriched", Summary: "renamed-summary",
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, word := range []string{"zorblax", "quillfeather", "snarfwidget", "renamed-summary"} {
		var found bool
		if err := pool.QueryRow(ctx, "SELECT bigram @@ to_tsquery('simple', $2) FROM search_documents WHERE skill_id = $1", mustUUID(t, skillID), word).Scan(&found); err != nil || !found {
			t.Errorf("after IndexSkill the bigram column does not match %q (err=%v)", word, err)
		}
	}
}
