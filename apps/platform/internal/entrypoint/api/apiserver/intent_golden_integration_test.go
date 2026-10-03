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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"gopkg.in/yaml.v3"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
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

type goldenQuery struct {
	ID         string   `json:"id"`
	Query      string   `json:"query"`
	Lang       string   `json:"lang"`
	Primary    []string `json:"gold_primary"`
	Acceptable []string `json:"gold_acceptable"`
}

type goldenSearch struct {
	pool    *pgxpool.Pool
	ctx     context.Context
	root    string
	mode    string
	hybrid  bool
	vectors *goldenVectorReplay
	queries []goldenQuery
	replay  goldenIntentReplay
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
		loadGoldenVectors(t, root, vectors)
	}
	requireHistoricalEnglishBaseline(t, root)
	queries := readGoldenQueries(t, root)
	g := goldenSearch{
		pool: pool, ctx: ctx, root: root, mode: mode, hybrid: hybrid,
		vectors: vectors, queries: queries, replay: readGoldenIntentReplay(t, queries),
	}
	for _, corpus := range corpora {
		t.Run(corpus, func(t *testing.T) {
			g.measureCorpus(t, corpus)
		})
	}
}

func loadGoldenVectors(t *testing.T, root string, vectors *goldenVectorReplay) {
	t.Helper()
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

func requireHistoricalEnglishBaseline(t *testing.T, root string) {
	t.Helper()
	historicalResults, err := os.ReadFile(filepath.Join(root, "results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !historicalEnglishBaselinePresent(string(historicalResults)) {
		t.Fatal("historical English BM25 Top-1 baseline is missing or changed")
	}
}

func historicalEnglishBaselinePresent(results string) bool {
	_, historicalEnglish, found := strings.Cut(results, "### 語言 en")
	return found && strings.Contains(strings.SplitN(historicalEnglish, "###", 2)[0], "| BM25 | 14/18 (78%) |")
}

func readGoldenQueries(t *testing.T, root string) []goldenQuery {
	t.Helper()
	var queries struct {
		Queries []goldenQuery `json:"queries"`
	}
	readGoldenJSON(t, filepath.Join(root, "queries.json"), &queries)
	if len(queries.Queries) != 60 {
		t.Fatalf("queries=%d, want 60", len(queries.Queries))
	}
	return queries.Queries
}

func readGoldenIntentReplay(t *testing.T, queries []goldenQuery) goldenIntentReplay {
	t.Helper()
	var replay goldenIntentReplay
	path := os.Getenv("SKILLHUB_INTENT_SNAPSHOT")
	if path == "" {
		return replay
	}
	readGoldenJSON(t, path, &replay.Rows)
	if len(replay.Rows) != len(queries) {
		t.Fatalf("snapshot rows=%d, want %d", len(replay.Rows), len(queries))
	}
	for _, query := range queries {
		if matches := replay.matches(query.Query); matches != 1 {
			t.Fatalf("query %s has %d snapshot matches", query.ID, matches)
		}
	}
	return replay
}

type goldenCorpus struct {
	goldenSearch
	name        string
	owner       string
	workspaceID pgtype.UUID
	svc         *catalog.Service
	purpose     string
	poison      map[string]string
	ids         map[string]string
	versions    map[string]int
}

func (g goldenSearch) measureCorpus(t *testing.T, corpus string) {
	a := newAPI(t, g.pool)
	owner := a.login(t, uniqueWorklistLabel("golden-lexical"))
	c := &goldenCorpus{
		goldenSearch: g, name: corpus, owner: owner.workspaceID, workspaceID: mustUUID(t, owner.workspaceID),
		ids: map[string]string{}, versions: map[string]int{},
	}
	c.wireCatalogService()
	files := c.corpusFiles(t)
	for _, file := range files {
		c.indexFile(t, file)
	}
	indexed := c.requireIndexed(t, len(files))
	t.Logf("%s_INPUT corpus=%s indexed=%d poison=%d versions=%v", g.mode, corpus, indexed, len(c.poison), c.versions)
	counts := map[string]*intentGoldenTally{"zh": {}, "en": {}}
	handler := &catalog.Handler{Svc: c.svc}
	for _, query := range g.queries {
		c.scoreQuery(t, handler, query, counts)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Logf("%s_BASELINE corpus=%s language=%s counts=%+v", g.mode, corpus, lang, *counts[lang])
		count := counts[lang]
		total := float64(count.Queries + count.Distractors)
		t.Logf("%s_QUALITY corpus=%s language=%s f1_at_gold=%.4f recall_at_gold=%.4f poison_top3=%d", g.mode, corpus, lang, count.F1/total, count.Recall/total, count.PoisonTop3)
	}
	zh, en := counts["zh"], counts["en"]
	if zh.Queries == 0 || en.Queries == 0 {
		t.Fatal("empty language stratum")
	}
	if !g.hybrid {
		t.Logf("LEXICAL_FLOOR corpus=%s measured_only=true metric=top1 observed=%d/%d historical_english=14/18 passes_floor=%v", corpus, zh.Top1, zh.Queries, historicalLexicalFloorMet(zh.Top1, zh.Queries))
	}
}

func (c *goldenCorpus) wireCatalogService() {
	svc := wiring.NewCatalogService(c.pool)
	if c.hybrid {
		svc.LLM = c.vectors
	}
	purpose := "reference"
	if len(c.replay.Rows) > 0 {
		svc.IntentAnalyzer = c.replay
		purpose = ""
	}
	workspaceID := c.workspaceID
	svc.CatalogWorkspaces = func(context.Context, gen.DBTX) ([]pgtype.UUID, error) {
		return []pgtype.UUID{workspaceID}, nil
	}
	c.svc, c.purpose = svc, purpose
}

func (c *goldenCorpus) corpusFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(c.root, c.name, "*", "*.json"))
	if err != nil || len(files) != 31 {
		t.Fatalf("corpus files=%d err=%v", len(files), err)
	}
	c.poison = map[string]string{}
	if os.Getenv("SKILLHUB_INTENT_POISON") == "1" {
		c.poison = map[string]string{
			"everything-office-helper":    "documents",
			"universal-writing-assistant": "writing",
			"all-data-tasks":              "data",
		}
		poison, err := filepath.Glob(filepath.Join(c.root, "..", "..", "docs", "plans", "mvp", "m5", "creation-measure", "injection", "poison-enriched", "*.json"))
		if err != nil || len(poison) != len(c.poison) {
			t.Fatalf("poison files=%d err=%v", len(poison), err)
		}
		files = append(files, poison...)
	}
	return files
}

type goldenTaskExample struct {
	Zh string `json:"zh_hant"`
	En string `json:"en"`
}

type goldenEnrichedSkill struct {
	Summary  string              `json:"summary"`
	Model    string              `json:"model"`
	Version  string              `json:"prompt_version"`
	Tags     json.RawMessage     `json:"tags"`
	Examples []goldenTaskExample `json:"task_examples"`
}

type goldenFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func (c *goldenCorpus) indexFile(t *testing.T, file string) {
	t.Helper()
	var enriched goldenEnrichedSkill
	readGoldenJSON(t, file, &enriched)
	c.versions[enriched.Model+"/"+enriched.Version]++
	stem, category, source := goldenSource(c.root, file, c.poison)
	front := readGoldenFrontmatter(t, source, stem)
	if front.Name == "" || front.Description == "" || enriched.Summary == "" {
		t.Fatalf("incomplete corpus row: %s", stem)
	}
	id := seedSkill(t, c.pool, c.owner, front.Name)
	seedSkillVersion(t, c.pool, c.owner, id)
	setCategory(t, c.pool, id, category)
	c.ids[id] = stem
	examples := goldenTaskExamples(enriched.Examples)
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		t.Fatal(err)
	}
	projection := catalog.EnrichedSkillProjection{
		SkillID: mustUUID(t, id), WorkspaceID: c.workspaceID, Name: front.Name,
		Summary: front.Description, EnrichedSummary: enriched.Summary,
		TaskExamples: strings.Join(examples, "\n"), Tags: enriched.Tags, Scan: []byte(`{}`),
		EnrichmentStatus: "enriched", EnrichmentModel: &enriched.Model, EnrichmentPromptVersion: &enriched.Version,
	}
	if c.hybrid {
		text := ingest.EmbeddingText(front.Name, front.Description, enriched.Summary, projection.TaskExamples, enriched.Tags)
		vector := pgvector.NewVector(c.vectors.lookup(text))
		projection.Embedding = &vector
	}
	err = c.svc.IndexSkillEnriched(c.ctx, tx, projection)
	if err != nil {
		_ = tx.Rollback(c.ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(c.ctx); err != nil {
		t.Fatal(err)
	}
}

func goldenSource(root, file string, poison map[string]string) (stem, category, source string) {
	stem = strings.TrimSuffix(filepath.Base(file), ".json")
	category = filepath.Base(filepath.Dir(file))
	source = filepath.Join(root, "corpus", category, stem+".md")
	if poisonCategory, ok := poison[stem]; ok {
		category = poisonCategory
		source = filepath.Join(filepath.Dir(file), "..", "poison", stem+".md")
	}
	return stem, category, source
}

func readGoldenFrontmatter(t *testing.T, source, stem string) goldenFrontmatter {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	head, ok := goldenFrontmatterBlock(string(raw))
	if !ok {
		t.Fatalf("missing frontmatter: %s", stem)
	}
	var front goldenFrontmatter
	if err := yaml.Unmarshal([]byte(head), &front); err != nil {
		t.Fatal(err)
	}
	return front
}

func goldenFrontmatterBlock(raw string) (string, bool) {
	parts := strings.SplitN(strings.ReplaceAll(raw, "\r\n", "\n"), "\n---", 2)
	if len(parts) != 2 {
		return "", false
	}
	return parts[0], true
}

func goldenTaskExamples(examples []goldenTaskExample) []string {
	var texts []string
	for _, example := range examples {
		for _, text := range []string{example.Zh, example.En} {
			if text = strings.TrimSpace(text); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return texts
}

func (c *goldenCorpus) requireIndexed(t *testing.T, files int) int {
	t.Helper()
	var indexed int
	if err := c.pool.QueryRow(c.ctx, `SELECT count(*) FROM search_documents WHERE workspace_id = $1 AND listable AND category IN ('documents', 'writing', 'data') AND enriched_summary <> '' AND bigram <> ''::tsvector`, c.workspaceID).Scan(&indexed); err != nil || indexed != files {
		t.Fatalf("indexed rows=%d want=%d err=%v", indexed, files, err)
	}
	if c.hybrid {
		var embedded int
		if err := c.pool.QueryRow(c.ctx, `SELECT count(*) FROM search_documents WHERE workspace_id = $1 AND embedding IS NOT NULL`, c.workspaceID).Scan(&embedded); err != nil || embedded != files {
			t.Fatalf("embedded=%d want=%d err=%v", embedded, files, err)
		}
	}
	return indexed
}

type catalogSearchBody struct {
	Interpretation catalog.SearchInterpretation `json:"interpretation"`
	Degraded       bool                         `json:"degraded"`
	Results        []searchResultRef            `json:"results"`
}

type searchResultRef struct {
	ID string `json:"skill_id"`
}

func searchResultsContain(results []searchResultRef, id string) bool {
	found := false
	for _, result := range results {
		found = found || result.ID == id
	}
	return found
}

func (c *goldenCorpus) scoreQuery(t *testing.T, handler *catalog.Handler, query goldenQuery, counts map[string]*intentGoldenTally) {
	t.Helper()
	beforeCalls := c.vectors.calls
	w := httptest.NewRecorder()
	handler.PublicSearch(w, httptest.NewRequest(http.MethodGet, "/api/skills/search?purpose="+c.purpose+"&limit=5&q="+url.QueryEscape(query.Query), nil))
	var body catalogSearchBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Degraded == c.hybrid {
		t.Fatalf("query=%s status=%d body=%s", query.ID, w.Code, w.Body.String())
	}
	if c.hybrid && c.vectors.calls != beforeCalls+1 {
		t.Fatalf("query=%s embedding calls=%d want=1", query.ID, c.vectors.calls-beforeCalls)
	}
	if len(c.replay.Rows) > 0 {
		if expected := c.replay.expectedStatus(query.Query); body.Interpretation.Status != expected {
			t.Fatalf("query=%s interpretation=%s want=%s", query.ID, body.Interpretation.Status, expected)
		}
	}
	results := c.resultStems(t, query, body.Results)
	relevant := append(slices.Clone(query.Primary), query.Acceptable...)
	if gold, absent := absentGold(relevant, c.ids); absent {
		t.Fatalf("query %s references absent gold %s", query.ID, gold)
	}
	count := counts[query.Lang]
	if count == nil {
		t.Fatalf("unknown query language %s", query.Lang)
	}
	if poisonInTopThree(results, c.poison) {
		count.PoisonTop3++
	}
	count.score(relevant, results)
	t.Logf("%s_QUERY corpus=%s id=%s lang=%s results=%v gold=%v interpretation=%+v", c.mode, c.name, query.ID, query.Lang, results, relevant, body.Interpretation)
}

func (c *goldenCorpus) resultStems(t *testing.T, query goldenQuery, found []searchResultRef) []string {
	t.Helper()
	var results []string
	for _, result := range found {
		id, ok := c.ids[result.ID]
		if !ok {
			t.Fatalf("query %s leaked a foreign corpus row %s", query.ID, result.ID)
		}
		results = append(results, id)
	}
	return results
}

func absentGold(relevant []string, ids map[string]string) (string, bool) {
	for _, gold := range relevant {
		found := false
		for _, id := range ids {
			found = found || id == gold
		}
		if !found {
			return gold, true
		}
	}
	return "", false
}

func poisonInTopThree(results []string, poison map[string]string) bool {
	for _, id := range results[:min(3, len(results))] {
		if _, poisoned := poison[id]; poisoned {
			return true
		}
	}
	return false
}

type intentGoldenTally struct {
	Queries, Top1, Top3, Recall5, Distractors, Rejected, PoisonTop3 int
	F1, Recall                                                      float64
}

func (count *intentGoldenTally) score(relevant, results []string) {
	if len(relevant) == 0 {
		count.Distractors++
		if len(results) == 0 {
			count.Rejected++
			count.F1++
			count.Recall++
		}
		return
	}
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

type goldenIntentRow struct {
	Query    string `json:"query"`
	Response *struct {
		Valid bool `json:"valid"`
		catalog.SearchInterpretation
	} `json:"response"`
}

type goldenIntentReplay struct {
	Rows []goldenIntentRow
}

func (r goldenIntentReplay) matches(query string) int {
	matches := 0
	for _, row := range r.Rows {
		if row.Query == query {
			matches++
		}
	}
	return matches
}

func (r goldenIntentReplay) expectedStatus(query string) string {
	expected := "fallback"
	for _, row := range r.Rows {
		if row.Query == query && row.Response != nil && row.Response.Valid {
			expected = "analyzed"
		}
	}
	return expected
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
