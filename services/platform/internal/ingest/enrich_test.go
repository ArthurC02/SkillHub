package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

const (
	testDescription     = "Work with PDFs."
	testEnrichedSummary = "Reads PDF files and pulls the tables out of them."
	testExampleZh       = "幫我從掃描的檔案裡抽出表格資料"
	testExampleEn       = "extract the tables from this scanned document"
	testLimitation      = "無法處理加密的 PDF。"
)

// stubEnricher stands in for the Python LLM service. enrichStatus/embedStatus
// other than 200 make that endpoint fail, which is how the fallback chain is
// exercised. The last text sent to /embed is captured so a test can assert what
// actually got indexed.
type stubEnricher struct {
	enrichStatus int
	embedStatus  int
	embedded     []string
}

func (s *stubEnricher) start(t *testing.T) *llmclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrich-skill", func(w http.ResponseWriter, _ *http.Request) {
		if s.enrichStatus != http.StatusOK {
			http.Error(w, `{"detail":"gateway error"}`, s.enrichStatus)
			return
		}
		writeTestJSON(w, llmclient.EnrichSkillResponse{
			Summary:      testEnrichedSummary,
			TaskExamples: []llmclient.TaskExample{{ZhHant: testExampleZh, En: testExampleEn}},
			Tags: llmclient.SkillTags{
				Inputs:       []string{"pdf"},
				Outputs:      []string{"csv"},
				Dependencies: []string{"poppler"},
			},
			Limitations:   []string{testLimitation},
			Model:         "gpt-5.6-sol",
			PromptVersion: "enrich-skill/v2",
		})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, r *http.Request) {
		var req llmclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		s.embedded = req.Texts
		if s.embedStatus != http.StatusOK {
			http.Error(w, `{"detail":"gateway error"}`, s.embedStatus)
			return
		}
		writeTestJSON(w, llmclient.EmbedResponse{
			Embeddings: [][]float32{make([]float32, 1536)},
			Model:      "text-embedding-3-small",
			Dimensions: 1536,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func testPackage() preparedPackage {
	return preparedPackage{
		report: skillpkg.Report{Manifest: &skillpkg.Manifest{
			Name:        "pdf-tools",
			Description: testDescription,
		}},
		skillMD:  skillMD,
		fileTree: []string{"SKILL.md"},
	}
}

// A successful enrichment fills the projection and marks the document usable by
// the vector leg (ADR-013 §1).
func TestEnrichPackageStoresGeneratedFieldsAndEmbedding(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	s := &Service{LLM: stub.start(t)}

	e := s.enrichPackage(context.Background(), testPackage())

	if e.status != enrichmentEnriched {
		t.Fatalf("status = %q, want %q", e.status, enrichmentEnriched)
	}
	if e.embedding == nil {
		t.Fatal("no embedding stored; the document would be invisible to the vector leg")
	}
	if e.enrichedSummary != testEnrichedSummary {
		t.Fatalf("enriched_summary = %q, want the model's summary", e.enrichedSummary)
	}
	// The frontmatter description is kept alongside, never replaced: it is the
	// one field on the row that is not model-generated.
	if e.summary != testDescription {
		t.Fatalf("summary = %q, want the frontmatter description %q", e.summary, testDescription)
	}
	// Provenance is what lets a reader tell generated text from declared text
	// and lets a prompt change find its stale rows (ADR-013).
	if e.model == nil || *e.model != "gpt-5.6-sol" {
		t.Fatalf("model provenance = %v, want gpt-5.6-sol", e.model)
	}
	if e.promptVersion == nil || *e.promptVersion != "enrich-skill/v2" {
		t.Fatalf("prompt_version provenance = %v", e.promptVersion)
	}
	if !strings.Contains(e.taskExamples, testExampleZh) || !strings.Contains(e.taskExamples, testExampleEn) {
		t.Fatalf("task_examples dropped a language: %q", e.taskExamples)
	}
	if e.limitations != testLimitation {
		t.Fatalf("limitations = %q, want the model's line (DISC-003 限制)", e.limitations)
	}
	// The buckets have to survive storage: DISC-002 shows 依賴 on a result row
	// and DISC-003 shows 輸入/輸出/依賴 apart, and a flattened list cannot say
	// which token was which.
	var tags llmclient.SkillTags
	if err := json.Unmarshal(e.tags, &tags); err != nil {
		t.Fatalf("tags are not valid json: %v (%q)", err, e.tags)
	}
	if len(tags.Inputs) != 1 || tags.Inputs[0] != "pdf" ||
		len(tags.Dependencies) != 1 || tags.Dependencies[0] != "poppler" {
		t.Fatalf("tag buckets = %+v, want them kept apart", tags)
	}
}

// The scan facts are projected for every import, enriched or not: they come
// from the package, not from the model, so an unreachable LLM service must not
// cost a result row its risk hint (DISC-002 風險提示).
func TestScanFactsProjectedWithoutLLM(t *testing.T) {
	p := testPackage()
	p.report.Findings = []skillpkg.Finding{
		{Severity: skillpkg.SeverityWarning, Code: "embedded-script"},
		{Severity: skillpkg.SeverityWarning, Code: "license-unknown"},
		{Severity: skillpkg.SeverityInfo, Code: "script-file"},
		{Severity: skillpkg.SeverityInfo, Code: "script-file"},
	}
	s := &Service{}

	e := s.enrichPackage(context.Background(), p)

	var got scanFacts
	if err := json.Unmarshal(e.scan, &got); err != nil {
		t.Fatalf("scan facts are not valid json: %v (%q)", err, e.scan)
	}
	if got.Warnings != 2 {
		t.Fatalf("warnings = %d, want 2", got.Warnings)
	}
	if len(got.Codes) != 3 {
		t.Fatalf("codes = %v, want each code once", got.Codes)
	}
}

// golden-query-set.md §3.5/§3.6: the vector is computed over the enriched
// summary plus the task example sentences, not over the raw package. Both
// remaining recall@5 misses in that measurement were traced to queries that
// only the example sentences could have matched, so if the examples stop
// reaching the embedding call the fix silently reverts.
func TestEmbeddingCoversEnrichedSummaryAndTaskExamples(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
	s := &Service{LLM: stub.start(t)}

	s.enrichPackage(context.Background(), testPackage())

	if len(stub.embedded) != 1 {
		t.Fatalf("embed called with %d texts, want 1", len(stub.embedded))
	}
	text := stub.embedded[0]
	for _, want := range []string{"pdf-tools", testEnrichedSummary, testExampleZh, testExampleEn} {
		if !strings.Contains(text, want) {
			t.Errorf("embedded text is missing %q:\n%s", want, text)
		}
	}
	// The package body is deliberately not in there: indexing full text measured
	// 21 points worse at Top-1 than indexing the summary (§3.5).
	if strings.Contains(text, "# PDF") {
		t.Errorf("embedded text includes the package body:\n%s", text)
	}
}

// A failed enrichment must not fail the import. The document falls back to the
// frontmatter description and is marked pending for the backfill.
func TestEnrichPackageFallsBackToPendingWhenEnrichFails(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusBadGateway, embedStatus: http.StatusOK}
	s := &Service{LLM: stub.start(t)}

	e := s.enrichPackage(context.Background(), testPackage())

	if e.status != enrichmentPending {
		t.Fatalf("status = %q, want %q", e.status, enrichmentPending)
	}
	if e.summary != testDescription {
		t.Fatalf("summary = %q, want the frontmatter description as fallback", e.summary)
	}
	if e.embedding != nil || e.enrichedSummary != "" || e.model != nil {
		t.Fatal("a failed enrichment left generated fields behind")
	}
	if len(stub.embedded) != 0 {
		t.Fatal("embedded text that was never enriched")
	}
}

// Enrichment succeeded but the embedding did not. The generated text is worth
// keeping, but with no vector the document cannot be ranked, so it stays
// pending — one flag covers both halves of "needs a rebuild".
func TestEnrichPackageStaysPendingWhenEmbedFails(t *testing.T) {
	stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusBadGateway}
	s := &Service{LLM: stub.start(t)}

	e := s.enrichPackage(context.Background(), testPackage())

	if e.status != enrichmentPending {
		t.Fatalf("status = %q, want %q", e.status, enrichmentPending)
	}
	if e.embedding != nil {
		t.Fatal("embedding stored despite the embed call failing")
	}
	if e.enrichedSummary != testEnrichedSummary {
		t.Fatal("the usable generated summary was thrown away with the failed embedding")
	}
}

// No LLM service configured at all: import still works, document is pending.
func TestEnrichPackageWithoutLLMIsPending(t *testing.T) {
	s := &Service{}

	e := s.enrichPackage(context.Background(), testPackage())

	if e.status != enrichmentPending {
		t.Fatalf("status = %q, want %q", e.status, enrichmentPending)
	}
	if e.summary != testDescription {
		t.Fatalf("summary = %q, want the frontmatter description", e.summary)
	}
}

// readPackage feeds the enrichment call: without SKILL.md and the file tree the
// model is describing nothing.
func TestReadPackageCollectsEnrichmentInputs(t *testing.T) {
	p, err := readPackage(zipBytes(t, map[string]string{
		"SKILL.md":         skillMD,
		"scripts/split.py": "print('hi')",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.skillMD, "name: pdf-tools") {
		t.Fatalf("skillMD not captured: %q", p.skillMD)
	}
	if len(p.fileTree) != 2 {
		t.Fatalf("fileTree = %v, want both files", p.fileTree)
	}
}
