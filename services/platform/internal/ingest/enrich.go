package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// Enrichment status values stored on the search document (0011).
const (
	// enrichmentPending: the projection has no usable vector for this skill, so
	// it cannot be ranked by the vector leg. Cleared by a successful enrichment,
	// which is either the next import or a manual `reindex` backfill. Retrying
	// is a Go decision (iron rule 6) and this flag is the queue it reads; there
	// is deliberately no automatic retry schedule in this batch.
	enrichmentPending = "pending"
	// enrichmentEnriched: model text and its embedding both landed.
	enrichmentEnriched = "enriched"
)

// Timeouts for the two index-time calls. Go owns timeout policy, not Python
// (iron rule 6); the enrich budget sits just above the LLM service's own 60s
// ceiling so its error surfaces here instead of our deadline.
const (
	enrichTimeout = 75 * time.Second
	embedTimeout  = 20 * time.Second
)

// enrichment is what the search projection stores for one skill version.
// Summary and scan are always populated (they come from the package itself);
// everything model-written is empty on the pending path.
type enrichment struct {
	summary         string // frontmatter description — never model-generated
	enrichedSummary string
	taskExamples    string
	tags            []byte // SkillTags as jsonb; buckets kept apart (DISC-002/003)
	limitations     string
	scan            []byte // scanFacts as jsonb
	embedding       *pgvector.Vector
	status          string
	model           *string
	promptVersion   *string
}

// scanFacts is the static-scan summary the search projection carries so a
// result row can show a risk hint without re-reading the package (DISC-002
// 風險提示). The detail view re-scans instead and reports the findings verbatim;
// this is deliberately the lossy version, sized for a list row.
//
// Errors are not counted: an error-level finding blocks the import, so nothing
// carrying one reaches the projection.
type scanFacts struct {
	Warnings int      `json:"warnings"`
	Codes    []string `json:"codes"` // distinct finding codes, warning and info alike
}

// scanFactsFrom folds a validation report into the projected facts.
func scanFactsFrom(r skillpkg.Report) []byte {
	f := scanFacts{Codes: []string{}}
	seen := make(map[string]bool, len(r.Findings))
	for _, finding := range r.Findings {
		if finding.Severity == skillpkg.SeverityWarning {
			f.Warnings++
		}
		if !seen[finding.Code] {
			seen[finding.Code] = true
			f.Codes = append(f.Codes, finding.Code)
		}
	}
	b, err := json.Marshal(f)
	if err != nil {
		// A struct of an int and a string slice does not fail to marshal. If it
		// somehow does, a NULL scan is the honest answer: the reader reports
		// the scan as unavailable rather than as clean.
		slog.Warn("scan facts not serialisable; projecting no scan", "error", err)
		return nil
	}
	return b
}

// enrichPackage runs the ADR-013 §1 index-time enhancement for one package and
// returns what the projection should store. It never returns an error: every
// failure degrades to a pending document built from the frontmatter description,
// because a skill that imported successfully must be findable even when the
// model is not reachable.
//
// Called outside the import transaction on purpose — these are two network
// round-trips and holding a Postgres transaction open across them would pin a
// connection for the length of an LLM call.
func (s *Service) enrichPackage(ctx context.Context, p preparedPackage) enrichment {
	e := enrichment{
		summary: p.report.Manifest.Description,
		scan:    scanFactsFrom(p.report),
		status:  enrichmentPending,
	}
	if s.LLM == nil || p.skillMD == "" {
		return e
	}

	enrichCtx, cancel := context.WithTimeout(ctx, enrichTimeout)
	defer cancel()
	resp, err := s.LLM.EnrichSkill(enrichCtx, llmclient.EnrichSkillRequest{
		SkillName: p.report.Manifest.Name,
		SkillMD:   p.skillMD,
		FileTree:  p.fileTree,
	})
	if err != nil {
		slog.Warn("index-time enrichment failed; search document left pending",
			"skill", p.report.Manifest.Name, "error", err)
		return e
	}
	e.enrichedSummary = resp.Summary
	e.taskExamples = joinTaskExamples(resp.TaskExamples)
	e.tags = marshalTags(resp.Tags)
	e.limitations = joinLines(resp.Limitations)
	e.model, e.promptVersion = &resp.Model, &resp.PromptVersion

	embedCtx, cancelEmbed := context.WithTimeout(ctx, embedTimeout)
	defer cancelEmbed()
	emb, err := s.LLM.Embed(embedCtx, []string{embeddingText(p.report.Manifest.Name, e)})
	if err != nil || len(emb.Embeddings) == 0 {
		// The generated text is worth keeping — it is still the better display
		// summary — but without a vector this document cannot be ranked, so it
		// stays pending for the backfill.
		slog.Warn("enrichment embedding failed; search document left pending",
			"skill", p.report.Manifest.Name, "error", err)
		return e
	}
	v := pgvector.NewVector(emb.Embeddings[0])
	e.embedding = &v
	e.status = enrichmentEnriched
	return e
}

// embeddingText builds the string the vector leg indexes: the enriched summary
// and the example task sentences, not the package body.
//
// golden-query-set.md §3.5 measured a summary-shaped index at 77% Top-1 against
// 56% for full text over the same 31-document corpus — long bodies dilute the
// vector. §3.6 then traced both remaining recall@5 misses to the absence of the
// task example sentences specifically: a Traditional Chinese query about
// extracting tables from scans could not reach an English `pdf` SKILL.md whose
// frontmatter never says either thing. The examples are what closes that gap, so
// they belong in the embedded text and not only in the stored row.
func embeddingText(name string, e enrichment) string {
	body := e.enrichedSummary
	if body == "" {
		body = e.summary
	}
	parts := []string{name + ": " + body}
	if e.taskExamples != "" {
		parts = append(parts, e.taskExamples)
	}
	if tags := e.flatTags(); tags != "" {
		parts = append(parts, tags)
	}
	return strings.Join(parts, "\n")
}

// flatTags is every tag bucket as one space-separated line. Only the embedded
// text wants them flat — it is a bag of words, and which bucket a term came
// from carries no retrieval signal. Everything a reader sees keeps the buckets
// (DISC-002 依賴, DISC-003 輸入/輸出/依賴), which is why the stored column is
// jsonb and this collapses it rather than the other way round.
func (e enrichment) flatTags() string {
	var t llmclient.SkillTags
	if len(e.tags) == 0 || json.Unmarshal(e.tags, &t) != nil {
		return ""
	}
	var out []string
	for _, bucket := range [][]string{t.Inputs, t.Outputs, t.Tools, t.Dependencies} {
		out = append(out, bucket...)
	}
	return strings.Join(out, " ")
}

// joinTaskExamples flattens the bilingual examples one sentence per line. Both
// languages are kept: the corpus is English-dominant and the queries are not,
// so each side of the pair is the bridge for queries in the other language.
func joinTaskExamples(examples []llmclient.TaskExample) string {
	lines := make([]string, 0, len(examples)*2)
	for _, ex := range examples {
		lines = append(lines, ex.ZhHant, ex.En)
	}
	return joinLines(lines)
}

// joinLines is the projection's one-item-per-line convention, blanks dropped.
func joinLines(items []string) string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

// marshalTags stores the tag buckets as they came back. They used to be
// space-joined into one string on the reasoning that nothing read them apart;
// DISC-002 (依賴 on a result row) and DISC-003 (輸入/輸出/依賴 on the detail
// view) both do, and no amount of parsing recovers which token was which.
func marshalTags(t llmclient.SkillTags) []byte {
	for _, bucket := range []*[]string{&t.Inputs, &t.Outputs, &t.Tools, &t.Dependencies} {
		*bucket = trimAll(*bucket)
	}
	b, err := json.Marshal(t)
	if err != nil {
		slog.Warn("enrichment tags not serialisable; projecting none", "error", err)
		return nil
	}
	return b
}

// trimAll drops blank entries and returns a non-nil slice, so an empty bucket
// serialises as [] rather than null.
func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
