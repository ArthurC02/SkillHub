package ingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
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
// Summary is always populated; everything else is empty on the pending path.
type enrichment struct {
	summary         string // frontmatter description — never model-generated
	enrichedSummary string
	taskExamples    string
	tags            string
	embedding       *pgvector.Vector
	status          string
	model           *string
	promptVersion   *string
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
	e := enrichment{summary: p.report.Manifest.Description, status: enrichmentPending}
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
	e.tags = joinTags(resp.Tags)
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
	if e.tags != "" {
		parts = append(parts, e.tags)
	}
	return strings.Join(parts, "\n")
}

// joinTaskExamples flattens the bilingual examples one sentence per line. Both
// languages are kept: the corpus is English-dominant and the queries are not,
// so each side of the pair is the bridge for queries in the other language.
func joinTaskExamples(examples []llmclient.TaskExample) string {
	lines := make([]string, 0, len(examples)*2)
	for _, ex := range examples {
		for _, s := range []string{ex.ZhHant, ex.En} {
			if s = strings.TrimSpace(s); s != "" {
				lines = append(lines, s)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// joinTags flattens every tag bucket into one space-separated string. The
// buckets are not kept apart because nothing reads them apart: they exist here
// as retrieval signal, and DISC-003's structured filters will read the manifest
// rather than this projection.
func joinTags(t llmclient.SkillTags) string {
	var out []string
	for _, bucket := range [][]string{t.Inputs, t.Outputs, t.Tools, t.Dependencies} {
		for _, tag := range bucket {
			if tag = strings.TrimSpace(tag); tag != "" {
				out = append(out, tag)
			}
		}
	}
	return strings.Join(out, " ")
}
