package catalog

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreationKnowledgeAgainstLexicalOnTheDevCatalog(t *testing.T) {
	dbURL := os.Getenv("SKILLHUB_COMPARE_DATABASE_URL")
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	corpusPath := os.Getenv("CREATION_MEASURE_CORPUS")
	if dbURL == "" || base == "" || corpusPath == "" {
		t.Skip("SKILLHUB_COMPARE_DATABASE_URL, SKILLHUB_E2E_LLM_URL and CREATION_MEASURE_CORPUS select this measurement")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Reference []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"reference"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Reference) > 10 {
		corpus.Reference = corpus.Reference[:10]
	}
	s := &Service{Pool: pool, LLM: &llmclient.Client{BaseURL: base, Token: os.Getenv("LLM_SERVICE_TOKEN")}}
	names := func(hits []searchResult) []string {
		out := []string{}
		for i, h := range hits {
			if i == 3 {
				break
			}
			out = append(out, h.Name)
		}
		return out
	}
	overlapTotal, degraded := 0, 0
	for _, task := range corpus.Reference {
		lex, _, err := s.ftsOnlySearch(ctx, gen.New(s.Pool), task.Description, 3, searchFilters{})
		if err != nil {
			t.Fatal(err)
		}
		embedding, err := s.embedQuery(ctx, task.Description)
		if err != nil {
			degraded++
			t.Logf("%s: embedding failed (%v); lexical=%v", task.ID, err, names(lex))
			continue
		}
		sem, _, err := s.hybridSearch(ctx, gen.New(s.Pool), task.Description, embedding, 3, searchFilters{}, MaxCosineDistance)
		if err != nil {
			t.Fatal(err)
		}
		l, m := names(lex), names(sem)
		overlap := 0
		for _, a := range l {
			for _, b := range m {
				if a == b {
					overlap++
				}
			}
		}
		overlapTotal += overlap
		t.Logf("%s: lexical=%s | semantic=%s | overlap=%d", task.ID, strings.Join(l, ", "), strings.Join(m, ", "), overlap)
	}
	t.Logf("tasks=%d embedding_failures=%d overlap_total=%d", len(corpus.Reference), degraded, overlapTotal)
}
