package apiserver_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestAFilteredSearchFindsAMatchPastTheUnfilteredCandidateCut(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	curator := newAPI(t, pool).login(t, "curator-filter-cut")
	markCatalog(t, pool, curator.workspaceID)
	const axis, other = 1500, 1501
	for i := 0; i < 51; i++ {
		seedEmbedding(t, pool, seedSkill(t, pool, curator.workspaceID, fmt.Sprintf("filter-cut-decoy-%d", i)), axis)
	}
	wanted := seedSkill(t, pool, curator.workspaceID, "filter-cut-curated")
	seedBlendedEmbedding(t, pool, wanted, axis, other, 0.9)
	if _, err := pool.Exec(ctx, "UPDATE search_documents SET curated = true WHERE skill_id = $1", mustUUID(t, wanted)); err != nil {
		t.Fatal(err)
	}
	anon := &client{Client: http.DefaultClient, base: newAPIWithLLM(t, pool, stubLLM(t, axis, "because it fits")).URL}

	body := anon.search(t, "/api/skills/search?q=zxqvrarity&tier=curated")

	if ids := body.ids(); len(ids) != 1 || ids[0] != wanted || body.FilteredOut {
		t.Fatalf("tier=curated returned %v (filtered_out=%v), want only %s", ids, body.FilteredOut, wanted)
	}
}
