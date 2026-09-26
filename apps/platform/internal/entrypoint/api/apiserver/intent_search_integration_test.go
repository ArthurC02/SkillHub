package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestModelKeywordsExpandCandidatesBeyondTheVectorWindow(t *testing.T) {
	pool := requireDB(t)
	const query = "請整理收支資料"
	var available atomic.Bool
	var analyses atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
		analyses.Add(1)
		writeJSON(w, map[string]any{
			"valid": available.Load(), "model": "intent-test", "prompt_version": "search-intent/v2",
			"intent":   map[string]any{"input": "收支資料", "output": nil, "tools": nil, "data": nil, "environment": nil},
			"keywords": []string{"spreadsheet"}, "filters": map[string]string{},
		})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1492)}, "model": "test-embedding", "dimensions": embedDims})
	})
	model := httptest.NewServer(mux)
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	owner := a.login(t, uniqueWorklistLabel("keyword-window"))
	markCatalog(t, pool, owner.workspaceID)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `UPDATE workspaces SET is_catalog = false WHERE id = $1`, mustUUID(t, owner.workspaceID)); err != nil {
			t.Error(err)
		}
	})
	for range 50 {
		id := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("ledger"))
		seedEmbedding(t, pool, id, 1492)
	}
	target := seedSkill(t, pool, owner.workspaceID, "spreadsheet")
	seedBlendedEmbedding(t, pool, target, 1492, 1493, 0.8)
	anon := &client{Client: http.DefaultClient, base: a.URL}
	path := "/api/skills/search?q=" + url.QueryEscape(query) + "&limit=100"
	baseline := anon.search(t, path)
	if ids := baseline.ids(); baseline.Degraded || len(ids) != 50 || contains(ids, target) {
		t.Fatalf("vector window must exclude the weaker candidate: count=%d target=%v degraded=%v", len(ids), contains(ids, target), baseline.Degraded)
	}
	available.Store(true)
	expanded := anon.search(t, path)
	if ids := expanded.ids(); expanded.Degraded || len(ids) != 51 || ids[50] != target {
		t.Fatalf("keywords must add the candidate without promoting it: count=%d ids=%v degraded=%v", len(ids), ids, expanded.Degraded)
	}
	if analyses.Load() != 2 {
		t.Fatalf("analysis calls=%d, want one per search", analyses.Load())
	}
}

func TestAnalyzedSearchPreservesExactNamesAndUserTokenCoverage(t *testing.T) {
	pool := requireDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"valid": true, "model": "intent-test", "prompt_version": "search-intent/v2",
			"intent":   map[string]any{"input": nil, "output": nil, "tools": nil, "data": nil, "environment": nil},
			"keywords": []string{"spreadsheet"}, "filters": map[string]string{},
		})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1488)}, "model": "test-embedding", "dimensions": embedDims})
	})
	model := httptest.NewServer(mux)
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	owner := a.login(t, uniqueWorklistLabel("analyzed-coverage"))
	markCatalog(t, pool, owner.workspaceID)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `UPDATE workspaces SET is_catalog = false WHERE id = $1`, mustUUID(t, owner.workspaceID)); err != nil {
			t.Error(err)
		}
	})
	near := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("ledger"))
	seedEmbedding(t, pool, near, 1488)
	exactName := uniqueWorklistLabel("intent-pii")
	const coveredTerm = "紫麒麟票據尾碼"
	exact := seedSkill(t, pool, owner.workspaceID, exactName)
	seedEmbedding(t, pool, exact, 1489)
	covered := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("masker"))
	seedBlendedEmbedding(t, pool, covered, 1488, 1489, 0.8)
	for _, id := range []string{exact, covered} {
		if _, err := pool.Exec(context.Background(), `UPDATE search_documents SET bigram = to_tsvector('simple', $2) WHERE skill_id = $1`, mustUUID(t, id), catalog.LexicalIndexText(exactName+" "+coveredTerm)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{"exact name", exactName, []string{exact, covered, near}},
		{"covered Chinese term", coveredTerm, []string{covered, exact, near}},
		{"partial coverage", exactName + " 不存在", []string{near, covered}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape(tc.query))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body struct {
				Interpretation catalog.SearchInterpretation `json:"interpretation"`
				Results        []struct {
					ID string `json:"skill_id"`
				} `json:"results"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != 200 || body.Interpretation.Status != "analyzed" || len(body.Results) != len(tc.want) {
				t.Fatalf("status=%d body=%+v want=%v", res.StatusCode, body, tc.want)
			}
			for i, id := range tc.want {
				if body.Results[i].ID != id {
					t.Fatalf("result[%d]=%s want %s", i, body.Results[i].ID, id)
				}
			}
		})
	}
}

func TestModelKeywordsDoNotReplaceTaskEmbeddingOrGrantCoveragePriority(t *testing.T) {
	pool := requireDB(t)
	const query = "請把收支資料轉成報告"
	var embeddings atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"valid": true, "model": "intent-test", "prompt_version": "search-intent/v2",
			"intent":   map[string]any{"input": "收支資料", "output": "報告", "tools": nil, "data": nil, "environment": nil},
			"keywords": []string{"spreadsheet"}, "filters": map[string]string{},
		})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		embeddings.Add(1)
		var body struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Texts) != 1 || body.Texts[0] != query {
			t.Errorf("embedding input=%v want original task", body.Texts)
		}
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1490)}, "model": "test-embedding", "dimensions": embedDims})
	})
	model := httptest.NewServer(mux)
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	owner := a.login(t, uniqueWorklistLabel("keyword-expansion"))
	markCatalog(t, pool, owner.workspaceID)
	near := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("ledger-report"))
	seedEmbedding(t, pool, near, 1490)
	distant := seedSkill(t, pool, owner.workspaceID, "spreadsheet")
	seedEmbedding(t, pool, distant, 1491)
	if _, err := pool.Exec(context.Background(), `UPDATE search_documents SET bigram = to_tsvector('simple', 'spreadsheet') WHERE skill_id = $1`, mustUUID(t, distant)); err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape(query))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Degraded       bool                         `json:"degraded"`
		Interpretation catalog.SearchInterpretation `json:"interpretation"`
		Results        []struct {
			ID string `json:"skill_id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || body.Degraded || body.Interpretation.Status != "analyzed" || embeddings.Load() != 1 || len(body.Results) != 1 || body.Results[0].ID != near {
		t.Fatalf("status=%d embeddings=%d body=%+v", res.StatusCode, embeddings.Load(), body)
	}
	if len(body.Interpretation.Keywords) != 1 || body.Interpretation.Keywords[0] != "spreadsheet" {
		t.Fatalf("model proposal disappeared: %+v", body.Interpretation)
	}
}

func TestAnonymousIntentAnalysisRecordsVersionedCostEvenForInvalidOutput(t *testing.T) {
	pool := requireDB(t)
	for _, tc := range []struct {
		name  string
		valid bool
		usage bool
	}{
		{"valid", true, true},
		{"invalid output", false, true},
		{"missing usage", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelName := uniqueWorklistLabel("intent-cost")
			var analyses atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
				analyses.Add(1)
				body := map[string]any{
					"valid": tc.valid, "model": modelName, "prompt_version": "search-intent/v1",
					"intent":   map[string]any{"input": "CSV", "output": nil, "tools": nil, "data": nil, "environment": nil},
					"keywords": []string{"CSV"}, "filters": map[string]string{},
				}
				if tc.usage {
					body["usage"] = map[string]any{"prompt_tokens": 100, "completion_tokens": 50, "cost_usd": 0.001, "cost_source": "gateway"}
				}
				writeJSON(w, body)
			})
			mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1494)}, "model": "test-embedding", "dimensions": embedDims})
			})
			model := httptest.NewServer(mux)
			t.Cleanup(model.Close)
			a := newAPITuned(t, pool, model.URL, func(d *apiserver.Deps) {
				d.Limits = httpx.NewRateLimiter(1, 1)
			})
			response, err := http.Get(a.URL + "/api/skills/search?q=CSV")
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Interpretation catalog.SearchInterpretation `json:"interpretation"`
			}
			err = json.NewDecoder(response.Body).Decode(&body)
			response.Body.Close()
			status := "analyzed"
			if !tc.valid {
				status = "fallback"
			}
			if err != nil || response.StatusCode != 200 || body.Interpretation.Status != status || analyses.Load() != 1 {
				t.Fatalf("status=%d interpretation=%+v calls=%d err=%v", response.StatusCode, body.Interpretation, analyses.Load(), err)
			}
			var count int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM cost_events WHERE model = $1 AND kind = 'search_intent'`, modelName).Scan(&count); err != nil || count != 1 {
				t.Fatalf("cost rows=%d err=%v", count, err)
			}
			var version, source string
			var promptTokens, completionTokens, micros int64
			var anonymous bool
			if err := pool.QueryRow(context.Background(), `SELECT prompt_version, prompt_tokens, completion_tokens, usd_micros, cost_source, workspace_id IS NULL AND user_id IS NULL FROM cost_events WHERE model = $1 AND kind = 'search_intent'`, modelName).Scan(&version, &promptTokens, &completionTokens, &micros, &source, &anonymous); err != nil {
				t.Fatal(err)
			}
			wantMicros, wantPrompt, wantCompletion, wantSource := int64(1000), int64(100), int64(50), "gateway"
			if !tc.usage {
				wantMicros, wantPrompt, wantCompletion, wantSource = 0, 0, 0, "estimated"
			}
			if version != "search-intent/v1" || !anonymous || micros != wantMicros || promptTokens != wantPrompt || completionTokens != wantCompletion || source != wantSource {
				t.Fatalf("cost=%s %d/%d tokens %d micros source=%s anonymous=%v", version, promptTokens, completionTokens, micros, source, anonymous)
			}
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				req, err := http.NewRequest(method, a.URL+"/api/skills/search?q=CSV", strings.NewReader(`{}`))
				if err != nil {
					t.Fatal(err)
				}
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
				if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" || analyses.Load() != 1 {
					t.Fatalf("%s rate limit status=%d calls=%d", method, res.StatusCode, analyses.Load())
				}
			}
		})
	}
}

func TestCorrectedSearchRequestBoundariesPrecedeModelCalls(t *testing.T) {
	pool := requireDB(t)
	var embeddings atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		embeddings.Add(1)
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1493)}, "model": "boundary-embedding", "dimensions": embedDims})
	})
	model := httptest.NewServer(mux)
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	const base = `{"query":"CSV","intent":{"input":null,"output":null,"tools":null,"data":null,"environment":null},"keywords":[],"filters":{}`
	for _, tc := range []struct {
		name   string
		body   string
		status int
		limit  int
	}{
		{"omitted limit defaults", base + "}", 200, 20},
		{"minimum limit", base + `,"limit":1}`, 200, 1},
		{"maximum limit", base + `,"limit":100}`, 200, 100},
		{"null limit", base + `,"limit":null}`, 400, 0},
		{"fractional limit", base + `,"limit":1.5}`, 400, 0},
		{"string limit", base + `,"limit":"20"}`, 400, 0},
		{"zero limit", base + `,"limit":0}`, 400, 0},
		{"over limit", base + `,"limit":101}`, 400, 0},
		{"body at byte limit", base + "}" + strings.Repeat(" ", 131072-len(base)-1), 200, 20},
		{"body one byte over", base + "}" + strings.Repeat(" ", 131073-len(base)-1), 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := embeddings.Load()
			response, err := http.Post(a.URL+"/api/skills/search", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var body struct {
				Limit int    `json:"limit"`
				Error string `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status || body.Limit != tc.limit {
				t.Fatalf("status=%d body=%+v", response.StatusCode, body)
			}
			wantCalls := int32(0)
			if tc.status == 200 {
				wantCalls = 1
			} else if body.Error == "" {
				t.Fatal("missing rejection reason")
			}
			if got := embeddings.Load() - before; got != wantCalls {
				t.Fatalf("embedding calls=%d, want %d", got, wantCalls)
			}
		})
	}
}

func TestAlwaysFailingRewriterStillRetrievesNonemptyVectorResults(t *testing.T) {
	pool := requireDB(t)
	for _, embeddingFails := range []bool{false, true} {
		name := "vector available"
		if embeddingFails {
			name = "vector also unavailable"
		}
		t.Run(name, func(t *testing.T) {
			var analyses, embeddings atomic.Int32
			const query = "請幫我分析收支資料"
			mux := http.NewServeMux()
			mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
				analyses.Add(1)
				http.Error(w, "always failing rewriter", http.StatusBadGateway)
			})
			mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
				embeddings.Add(1)
				var body struct {
					Texts []string `json:"texts"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Texts) != 1 || body.Texts[0] != query {
					t.Errorf("embedding input=%v", body.Texts)
				}
				if embeddingFails {
					http.Error(w, "embedding unavailable", http.StatusBadGateway)
					return
				}
				writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1497)}, "model": "test-embedding", "dimensions": embedDims})
			})
			model := httptest.NewServer(mux)
			t.Cleanup(model.Close)
			a := newAPIWithLLM(t, pool, model.URL)
			curator := a.login(t, uniqueWorklistLabel("intent-fallback"))
			markCatalog(t, pool, curator.workspaceID)
			id := seedSkill(t, pool, curator.workspaceID, uniqueWorklistLabel("ledger-transformation"))
			seedEmbedding(t, pool, id, 1497)
			res, err := http.Get(a.URL + "/api/skills/search?q=" + url.QueryEscape(query))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body struct {
				Query          string                       `json:"query"`
				Degraded       bool                         `json:"degraded"`
				DegradedReason string                       `json:"degraded_reason"`
				NoResults      bool                         `json:"no_results"`
				Interpretation catalog.SearchInterpretation `json:"interpretation"`
				Results        []struct {
					SkillID string `json:"skill_id"`
				} `json:"results"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != 200 || analyses.Load() != 1 || embeddings.Load() != 1 || body.Query != query || body.Interpretation.Status != "fallback" {
				t.Fatalf("status=%d analyses=%d embeddings=%d response=%+v", res.StatusCode, analyses.Load(), embeddings.Load(), body)
			}
			if body.Degraded != embeddingFails {
				t.Fatalf("degraded=%v", body.Degraded)
			}
			if embeddingFails {
				if !body.NoResults || body.DegradedReason == "" || len(body.Results) != 0 {
					t.Fatalf("dishonest outage response=%+v", body)
				}
				return
			}
			found := false
			for _, result := range body.Results {
				if result.SkillID == id {
					found = true
				}
			}
			if !found || body.NoResults {
				t.Fatalf("vector-only match %s absent: %+v", id, body)
			}
		})
	}
}

func TestCorrectedPublicSearchUsesUserFieldsWithoutCallingRewriter(t *testing.T) {
	pool := requireDB(t)
	var analyses, embeddings, reasons atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/analyze-intent", func(w http.ResponseWriter, r *http.Request) {
		analyses.Add(1)
		http.Error(w, "must not rewrite correction", 500)
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		embeddings.Add(1)
		var body struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Texts) != 1 || body.Texts[0] != "invoice CSV" {
			t.Errorf("correction ignored: %v", body.Texts)
		}
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(1496)}, "model": "test-embedding", "dimensions": embedDims})
	})
	mux.HandleFunc("POST /match-reasons", func(w http.ResponseWriter, r *http.Request) {
		reasons.Add(1)
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Query != "invoice CSV" {
			t.Errorf("reasons ignore corrected fields: %q", body.Query)
		}
		writeJSON(w, map[string]any{"reasons": []any{}, "model": "test-reasons"})
	})
	model := httptest.NewServer(mux)
	t.Cleanup(model.Close)
	a := newAPIWithLLM(t, pool, model.URL)
	curator := a.login(t, uniqueWorklistLabel("intent-correction"))
	markCatalog(t, pool, curator.workspaceID)
	id := seedSkill(t, pool, curator.workspaceID, uniqueWorklistLabel("invoice-converter"))
	seedEmbedding(t, pool, id, 1496)
	res, err := http.Post(a.URL+"/api/skills/search", "application/json", strings.NewReader(`{"query":"原本的請求","intent":{"input":"invoice","output":"CSV","tools":null,"data":null,"environment":null},"keywords":["invoice"],"filters":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Query          string                       `json:"query"`
		Interpretation catalog.SearchInterpretation `json:"interpretation"`
		Results        []struct {
			SkillID string `json:"skill_id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || body.Query != "原本的請求" || body.Interpretation.Status != "corrected" || analyses.Load() != 0 || embeddings.Load() != 1 || reasons.Load() != 1 || len(body.Results) != 1 || body.Results[0].SkillID != id {
		t.Fatalf("status=%d analyses=%d embeddings=%d response=%+v", res.StatusCode, analyses.Load(), embeddings.Load(), body)
	}
}
