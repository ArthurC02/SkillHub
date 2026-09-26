package apiserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
	"gopkg.in/yaml.v3"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestGoldenLexicalBaselineUsesProductionProjectionAndQueries(t *testing.T) {
	measureGoldenSearch(t, false)
}

func TestRecordedIntentAnalysisUsesProductionValidationAndLexicalSearch(t *testing.T) {
	t.Setenv("SKILLHUB_INTENT_SNAPSHOT", filepath.Join("..", "..", "..", "..", "..", "..", "tools", "goldenset", "intent_analysis_v2.json"))
	measureGoldenSearch(t, false)
}

func TestGoldenHybridBaselineUsesRecordedVectorsAndProductionQueries(t *testing.T) {
	t.Setenv("SKILLHUB_INTENT_SNAPSHOT", "")
	measureGoldenSearch(t, true)
}

func TestRecordedIntentAnalysisUsesProductionValidationAndHybridSearch(t *testing.T) {
	t.Setenv("SKILLHUB_INTENT_SNAPSHOT", filepath.Join("..", "..", "..", "..", "..", "..", "tools", "goldenset", "intent_analysis_v2.json"))
	measureGoldenSearch(t, true)
}

func measureGoldenSearch(t *testing.T, hybrid bool) {
	t.Helper()
	pool := requireDB(t)
	ctx := context.Background()
	root := filepath.Join("..", "..", "..", "..", "..", "..", "tools", "goldenset")
	mode := "LEXICAL"
	corpora := []string{"corpus_enriched", "corpus_enriched_v7"}
	vectors := &goldenVectorReplay{t: t}
	if hybrid {
		mode = "HYBRID"
		readGoldenJSON(t, filepath.Join(root, "embeddings_cache.json"), &vectors.cache)
		var additional struct {
			Model   string               `json:"model"`
			Vectors map[string][]float32 `json:"vectors"`
		}
		readGoldenJSON(t, filepath.Join(root, "intent_embeddings_v2.json"), &additional)
		if additional.Model != "text-embedding-3-small" || len(additional.Vectors) != 94 {
			t.Fatal("incomplete additional embedding snapshot")
		}
		for key, vector := range additional.Vectors {
			if _, exists := vectors.cache[key]; exists {
				t.Fatalf("additional snapshot overwrites historical vector: %s", key)
			}
			vectors.cache[key] = vector
		}
	}
	historicalResults, err := os.ReadFile(filepath.Join(root, "results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, historicalEnglish, found := strings.Cut(string(historicalResults), "### 語言 en")
	if !found || !strings.Contains(strings.SplitN(historicalEnglish, "###", 2)[0], "| BM25 | 14/18 (78%) |") {
		t.Fatal("historical English BM25 Top-1 baseline is missing or changed")
	}
	var queries struct {
		Queries []struct {
			ID         string   `json:"id"`
			Query      string   `json:"query"`
			Lang       string   `json:"lang"`
			Primary    []string `json:"gold_primary"`
			Acceptable []string `json:"gold_acceptable"`
		} `json:"queries"`
	}
	readGoldenJSON(t, filepath.Join(root, "queries.json"), &queries)
	if len(queries.Queries) != 60 {
		t.Fatalf("queries=%d, want 60", len(queries.Queries))
	}
	var replay goldenIntentReplay
	if path := os.Getenv("SKILLHUB_INTENT_SNAPSHOT"); path != "" {
		readGoldenJSON(t, path, &replay.Rows)
		if len(replay.Rows) != len(queries.Queries) {
			t.Fatalf("snapshot rows=%d, want %d", len(replay.Rows), len(queries.Queries))
		}
		for _, query := range queries.Queries {
			matches := 0
			for _, row := range replay.Rows {
				if row.Query == query.Query {
					matches++
				}
			}
			if matches != 1 {
				t.Fatalf("query %s has %d snapshot matches", query.ID, matches)
			}
		}
	}
	for _, corpus := range corpora {
		t.Run(corpus, func(t *testing.T) {
			a := newAPI(t, pool)
			owner := a.login(t, uniqueWorklistLabel("golden-lexical"))
			workspaceID := mustUUID(t, owner.workspaceID)
			svc := wiring.NewCatalogService(pool)
			if hybrid {
				svc.LLM = vectors
			}
			purpose := "reference"
			if len(replay.Rows) > 0 {
				svc.IntentAnalyzer = replay
				purpose = ""
			}
			svc.CatalogWorkspaces = func(context.Context, gen.DBTX) ([]pgtype.UUID, error) {
				return []pgtype.UUID{workspaceID}, nil
			}
			files, err := filepath.Glob(filepath.Join(root, corpus, "*", "*.json"))
			if err != nil || len(files) != 31 {
				t.Fatalf("corpus files=%d err=%v", len(files), err)
			}
			poisonCategories := map[string]string{}
			if os.Getenv("SKILLHUB_INTENT_POISON") == "1" {
				poisonCategories = map[string]string{
					"everything-office-helper":    "documents",
					"universal-writing-assistant": "writing",
					"all-data-tasks":              "data",
				}
				poison, err := filepath.Glob(filepath.Join(root, "..", "..", "docs", "plans", "mvp", "m5", "creation-measure", "injection", "poison-enriched", "*.json"))
				if err != nil || len(poison) != len(poisonCategories) {
					t.Fatalf("poison files=%d err=%v", len(poison), err)
				}
				files = append(files, poison...)
			}
			ids := map[string]string{}
			versions := map[string]int{}
			for _, file := range files {
				var enriched struct {
					Summary  string          `json:"summary"`
					Model    string          `json:"model"`
					Version  string          `json:"prompt_version"`
					Tags     json.RawMessage `json:"tags"`
					Examples []struct {
						Zh string `json:"zh_hant"`
						En string `json:"en"`
					} `json:"task_examples"`
				}
				readGoldenJSON(t, file, &enriched)
				versions[enriched.Model+"/"+enriched.Version]++
				stem := strings.TrimSuffix(filepath.Base(file), ".json")
				category := filepath.Base(filepath.Dir(file))
				source := filepath.Join(root, "corpus", category, stem+".md")
				if poisonCategory, ok := poisonCategories[stem]; ok {
					category = poisonCategory
					source = filepath.Join(filepath.Dir(file), "..", "poison", stem+".md")
				}
				raw, err := os.ReadFile(source)
				if err != nil {
					t.Fatal(err)
				}
				parts := strings.SplitN(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n---", 2)
				if len(parts) != 2 {
					t.Fatalf("missing frontmatter: %s", stem)
				}
				var front struct {
					Name        string `yaml:"name"`
					Description string `yaml:"description"`
				}
				if err := yaml.Unmarshal([]byte(parts[0]), &front); err != nil {
					t.Fatal(err)
				}
				if front.Name == "" || front.Description == "" || enriched.Summary == "" {
					t.Fatalf("incomplete corpus row: %s", stem)
				}
				id := seedSkill(t, pool, owner.workspaceID, front.Name)
				seedSkillVersion(t, pool, owner.workspaceID, id)
				setCategory(t, pool, id, category)
				ids[id] = stem
				var examples []string
				for _, example := range enriched.Examples {
					for _, text := range []string{example.Zh, example.En} {
						if text = strings.TrimSpace(text); text != "" {
							examples = append(examples, text)
						}
					}
				}
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				projection := catalog.EnrichedSkillProjection{
					SkillID: mustUUID(t, id), WorkspaceID: workspaceID, Name: front.Name,
					Summary: front.Description, EnrichedSummary: enriched.Summary,
					TaskExamples: strings.Join(examples, "\n"), Tags: enriched.Tags, Scan: []byte(`{}`),
					EnrichmentStatus: "enriched", EnrichmentModel: &enriched.Model, EnrichmentPromptVersion: &enriched.Version,
				}
				if hybrid {
					var tags map[string][]string
					if err := json.Unmarshal(enriched.Tags, &tags); err != nil {
						t.Fatal(err)
					}
					parts := []string{front.Name + ": " + enriched.Summary}
					if len(examples) > 0 {
						parts = append(parts, strings.Join(examples, "\n"))
					}
					var flat []string
					for _, bucket := range []string{"inputs", "outputs", "tools", "dependencies"} {
						flat = append(flat, tags[bucket]...)
					}
					if len(flat) > 0 {
						parts = append(parts, strings.Join(flat, " "))
					}
					vector := pgvector.NewVector(vectors.lookup(strings.Join(parts, "\n")))
					projection.Embedding = &vector
				}
				err = svc.IndexSkillEnriched(ctx, tx, projection)
				if err != nil {
					_ = tx.Rollback(ctx)
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			var indexed int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_documents WHERE workspace_id = $1 AND listable AND category IN ('documents', 'writing', 'data') AND enriched_summary <> '' AND bigram <> ''::tsvector`, workspaceID).Scan(&indexed); err != nil || indexed != len(files) {
				t.Fatalf("indexed rows=%d want=%d err=%v", indexed, len(files), err)
			}
			if hybrid {
				var embedded int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_documents WHERE workspace_id = $1 AND embedding IS NOT NULL`, workspaceID).Scan(&embedded); err != nil || embedded != len(files) {
					t.Fatalf("embedded=%d want=%d err=%v", embedded, len(files), err)
				}
			}
			t.Logf("%s_INPUT corpus=%s indexed=%d poison=%d versions=%v", mode, corpus, indexed, len(poisonCategories), versions)
			type tally struct {
				Queries, Top1, Top3, Recall5, Distractors, Rejected, PoisonTop3 int
				F1, Recall                                                      float64
			}
			counts := map[string]*tally{"zh": {}, "en": {}}
			handler := &catalog.Handler{Svc: svc}
			for _, query := range queries.Queries {
				beforeCalls := vectors.calls
				w := httptest.NewRecorder()
				handler.PublicSearch(w, httptest.NewRequest(http.MethodGet, "/api/skills/search?purpose="+purpose+"&limit=5&q="+url.QueryEscape(query.Query), nil))
				var body struct {
					Interpretation catalog.SearchInterpretation `json:"interpretation"`
					Degraded       bool                         `json:"degraded"`
					Results        []struct {
						ID string `json:"skill_id"`
					} `json:"results"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if w.Code != 200 || body.Degraded == hybrid {
					t.Fatalf("query=%s status=%d body=%s", query.ID, w.Code, w.Body.String())
				}
				if hybrid && vectors.calls != beforeCalls+1 {
					t.Fatalf("query=%s embedding calls=%d want=1", query.ID, vectors.calls-beforeCalls)
				}
				if len(replay.Rows) > 0 {
					expected := "fallback"
					for _, row := range replay.Rows {
						if row.Query == query.Query && row.Response != nil && row.Response.Valid {
							expected = "analyzed"
						}
					}
					if body.Interpretation.Status != expected {
						t.Fatalf("query=%s interpretation=%s want=%s", query.ID, body.Interpretation.Status, expected)
					}
				}
				var results []string
				for _, result := range body.Results {
					id, ok := ids[result.ID]
					if !ok {
						t.Fatalf("query %s leaked a foreign corpus row %s", query.ID, result.ID)
					}
					results = append(results, id)
				}
				relevant := append(slices.Clone(query.Primary), query.Acceptable...)
				for _, gold := range relevant {
					found := false
					for _, id := range ids {
						found = found || id == gold
					}
					if !found {
						t.Fatalf("query %s references absent gold %s", query.ID, gold)
					}
				}
				count := counts[query.Lang]
				if count == nil {
					t.Fatalf("unknown query language %s", query.Lang)
				}
				for _, id := range results[:min(3, len(results))] {
					if _, poisoned := poisonCategories[id]; poisoned {
						count.PoisonTop3++
						break
					}
				}
				if len(relevant) == 0 {
					count.Distractors++
					if len(results) == 0 {
						count.Rejected++
						count.F1++
						count.Recall++
					}
				} else {
					count.Queries++
					page := results[:min(len(relevant), len(results))]
					hits := 0
					for _, id := range page {
						if slices.Contains(relevant, id) {
							hits++
						}
					}
					count.F1 += float64(2*hits) / float64(len(page)+len(relevant))
					count.Recall += float64(hits) / float64(len(relevant))
					for rank, id := range results {
						if !slices.Contains(relevant, id) {
							continue
						}
						if rank == 0 {
							count.Top1++
						}
						if rank < 3 {
							count.Top3++
						}
						count.Recall5++
						break
					}
				}
				t.Logf("%s_QUERY corpus=%s id=%s lang=%s results=%v gold=%v interpretation=%+v", mode, corpus, query.ID, query.Lang, results, relevant, body.Interpretation)
			}
			for _, lang := range []string{"zh", "en"} {
				t.Logf("%s_BASELINE corpus=%s language=%s counts=%+v", mode, corpus, lang, *counts[lang])
				count := counts[lang]
				total := float64(count.Queries + count.Distractors)
				t.Logf("%s_QUALITY corpus=%s language=%s f1_at_gold=%.4f recall_at_gold=%.4f poison_top3=%d", mode, corpus, lang, count.F1/total, count.Recall/total, count.PoisonTop3)
			}
			zh, en := counts["zh"], counts["en"]
			if zh.Queries == 0 || en.Queries == 0 {
				t.Fatal("empty language stratum")
			}
			if !hybrid {
				t.Logf("LEXICAL_FLOOR corpus=%s measured_only=true metric=top1 observed=%d/%d historical_english=14/18 passes_floor=%v", corpus, zh.Top1, zh.Queries, historicalLexicalFloorMet(zh.Top1, zh.Queries))
			}
		})
	}
}

type goldenVectorReplay struct {
	t     *testing.T
	cache map[string][]float32
	calls int
}

func (r *goldenVectorReplay) lookup(text string) []float32 {
	r.t.Helper()
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("text-embedding-3-small\n"+text)))
	vector, ok := r.cache[key]
	if !ok || len(vector) != embedDims {
		r.t.Fatalf("missing or invalid recorded embedding: key=%s dimensions=%d", key, len(vector))
	}
	var norm float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			r.t.Fatalf("non-finite recorded embedding: %s", key)
		}
		norm += float64(value) * float64(value)
	}
	if norm == 0 {
		r.t.Fatalf("zero recorded embedding: %s", key)
	}
	return slices.Clone(vector)
}

func (r *goldenVectorReplay) Embed(_ context.Context, texts []string, _ time.Duration) (*catalog.Embeddings, error) {
	r.calls++
	result := &catalog.Embeddings{Model: "text-embedding-3-small"}
	for _, text := range texts {
		result.Vectors = append(result.Vectors, r.lookup(text))
	}
	return result, nil
}

func (r *goldenVectorReplay) MatchReasons(context.Context, string, []catalog.SkillCandidate, time.Duration) (*catalog.MatchReasons, error) {
	return nil, errors.New("match reasons are outside retrieval quality replay")
}

func historicalLexicalFloorMet(hits, queries int) bool {
	return queries > 0 && hits*5 > queries && hits*18 >= queries*14
}

func TestHistoricalLexicalFloorUsesTopOneAndExactFractions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		hits, queries int
		meets         bool
	}{
		{"empty", 0, 0, false},
		{"previous Chinese", 6, 30, false},
		{"above previous Chinese but below English", 7, 30, false},
		{"just below required Chinese count", 23, 30, false},
		{"required Chinese count", 24, 30, true},
		{"below historical English", 13, 18, false},
		{"historical English equality", 14, 18, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := historicalLexicalFloorMet(tc.hits, tc.queries); got != tc.meets {
				t.Fatalf("Top-1=%d/%d floor=%v want=%v", tc.hits, tc.queries, got, tc.meets)
			}
		})
	}
}

type goldenIntentReplay struct {
	Rows []struct {
		Query    string `json:"query"`
		Response *struct {
			Valid bool `json:"valid"`
			catalog.SearchInterpretation
		} `json:"response"`
	}
}

func (r goldenIntentReplay) AnalyzeIntent(_ context.Context, query string, _ time.Duration) (*catalog.IntentAnalysis, error) {
	for _, row := range r.Rows {
		if row.Query == query && row.Response != nil {
			return &catalog.IntentAnalysis{Valid: row.Response.Valid, Interpretation: row.Response.SearchInterpretation}, nil
		}
	}
	return nil, errors.New("recorded intent analysis unavailable")
}

func readGoldenJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}
