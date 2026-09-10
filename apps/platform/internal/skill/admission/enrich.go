package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const (
	enrichmentPending = "pending"

	enrichmentEnriched = "enriched"
)

const (
	// budget-over: enrich.LLM_TIMEOUT_SECONDS
	enrichTimeout = 75 * time.Second

	// Set above the embedding service's own ceiling: the two clocks start at
	// different instants, so an equal value here would always expire first.
	// budget-over: app.EMBED_TIMEOUT_SECONDS
	embedTimeout = 25 * time.Second
)

type enrichment struct {
	summary         string
	enrichedSummary string
	taskExamples    string
	tags            []byte
	limitations     string
	scan            []byte
	embedding       *pgvector.Vector
	status          string
	model           *string
	promptVersion   *string
}

type scanFacts struct {
	Warnings int      `json:"warnings"`
	Codes    []string `json:"codes"`
}

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

		slog.Warn("scan facts not serialisable; projecting no scan", "error", err)
		return nil
	}
	return b
}

func (s *Service) enrichPackage(ctx context.Context, p preparedPackage, workspaceID pgtype.UUID) enrichment {
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

	s.recordCost(ctx, credit.KindIndexEnrich, workspaceID, resp.Model, resp.PromptVersion, resp.Usage)
	e.enrichedSummary = resp.Summary
	e.taskExamples = joinTaskExamples(resp.TaskExamples)
	e.tags = marshalTags(resp.Tags)
	e.limitations = joinLines(resp.Limitations)
	e.model, e.promptVersion = &resp.Model, &resp.PromptVersion

	for _, c := range resp.Checks {
		slog.Warn("enrichment disagrees with its own source document",
			"skill", p.report.Manifest.Name, "rule", c.Rule, "field", c.Field, "token", c.Token,
			"prompt_version", resp.PromptVersion)
	}

	embedCtx, cancelEmbed := context.WithTimeout(ctx, embedTimeout)
	defer cancelEmbed()
	emb, err := s.LLM.Embed(embedCtx, []string{embeddingText(p.report.Manifest.Name, e)})
	if emb != nil {
		s.recordCost(ctx, credit.KindIndexEnrich, workspaceID, emb.Model, "", emb.Usage)
	}
	if err != nil || len(emb.Embeddings) == 0 {

		slog.Warn("enrichment embedding failed; search document left pending",
			"skill", p.report.Manifest.Name, "error", err)
		return e
	}
	v := pgvector.NewVector(emb.Embeddings[0])
	e.embedding = &v
	e.status = enrichmentEnriched
	return e
}

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

func joinTaskExamples(examples []llmclient.TaskExample) string {
	lines := make([]string, 0, len(examples)*2)
	for _, ex := range examples {
		lines = append(lines, ex.ZhHant, ex.En)
	}
	return joinLines(lines)
}

func joinLines(items []string) string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

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

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
