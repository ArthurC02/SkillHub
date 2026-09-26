package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestARealGatewaySearchRecordsIntentAndEmbeddingCosts(t *testing.T) {
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	if base == "" {
		t.Skip("set SKILLHUB_E2E_LLM_URL to apps/llm with a bounded real Gateway key; this test spends money")
	}
	pool := requireDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	const query = "請將收支資料轉成報告"
	client := &llmclient.Client{BaseURL: base, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	embedding, err := client.EmbedWithin(ctx, []string{query}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Embeddings) != 1 || len(embedding.Embeddings[0]) != embedDims {
		t.Fatal("real embedding did not provide one full-dimensional fixture vector")
	}
	a := newAPIWithLLM(t, pool, base)
	owner := a.login(t, uniqueWorklistLabel("intent-live"))
	markCatalog(t, pool, owner.workspaceID)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `UPDATE workspaces SET is_catalog = false WHERE id = $1`, mustUUID(t, owner.workspaceID)); err != nil {
			t.Error(err)
		}
	})
	target := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("intent-live-target"))
	if _, err := pool.Exec(ctx, `UPDATE search_documents SET embedding = $2, listable = true WHERE skill_id = $1`, mustUUID(t, target), pgvector.NewVector(embedding.Embeddings[0])); err != nil {
		t.Fatal(err)
	}
	costs := func(kind string) (int64, int64) {
		t.Helper()
		var count, micros int64
		if err := pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(usd_micros), 0) FROM cost_events
			WHERE kind = $1 AND cost_source = 'gateway' AND workspace_id IS NULL AND user_id IS NULL
			AND ($1 <> 'search_intent' OR prompt_version = 'search-intent/v2')`, kind).Scan(&count, &micros); err != nil {
			t.Fatal(err)
		}
		return count, micros
	}
	intentCount, intentCost := costs("search_intent")
	embedCount, embedCost := costs("search_embedding")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/api/skills/search?q="+url.QueryEscape(query), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Query          string                       `json:"query"`
		Interpretation catalog.SearchInterpretation `json:"interpretation"`
		Degraded       bool                         `json:"degraded"`
		Results        []struct {
			ID string `json:"skill_id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	i := body.Interpretation
	if res.StatusCode != http.StatusOK || body.Query != query || body.Degraded || i.Status != "analyzed" || i.Model == "" || i.PromptVersion != "search-intent/v2" || len(i.Intent) != 5 {
		t.Fatalf("status=%d response=%+v", res.StatusCode, body)
	}
	for _, field := range []string{"input", "output", "tools", "data", "environment"} {
		value, exists := i.Intent[field]
		if !exists || (value != nil && (strings.TrimSpace(*value) == "" || !strings.Contains(query, *value))) {
			t.Fatalf("intent field %s is missing or invented", field)
		}
	}
	found := false
	for _, result := range body.Results {
		found = found || result.ID == target
	}
	if !found {
		t.Fatal("real search did not return the synthetic exact-vector fixture")
	}
	for _, before := range []struct {
		kind          string
		count, micros int64
	}{{"search_intent", intentCount, intentCost}, {"search_embedding", embedCount, embedCost}} {
		count, micros := costs(before.kind)
		if count != before.count+1 || micros <= before.micros {
			t.Fatalf("%s did not record exactly one positive, Gateway-reported anonymous cost", before.kind)
		}
		t.Logf("%s: Gateway-reported cost %d USD micros", before.kind, micros-before.micros)
	}
}
