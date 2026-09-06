package catalog

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/pgvector/pgvector-go"
)

// CreationReferenceIDs is a bounded lexical tool, without embedding, judge,
// analytics, or any other model spend outside the creation receipt.
func (s *Service) CreationReferenceIDs(ctx context.Context, query string) ([]string, error) {
	rows, _, err := s.ftsOnlySearch(ctx, gen.New(s.Pool), query, 10, searchFilters{})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.SkillID)
	}
	return ids, nil
}

// CreationMaxDistance is the creation tool's own cutoff, tighter than the
// public search's MaxCosineDistance. Measured on the 60-query golden set
// replayed through the product's SQL (creation-measure/search-f1, 2026-09-06):
// the lexical leg scores F1@3 0.02 (one top-1 hit in 48 task queries); the
// vector leg at 0.75 scores 0.61 with precision 0.47; at 0.55 it scores 0.77
// at one result and 0.75 at two, precision 0.92 / 0.74, and every distractor
// query is still rejected. The owner's rule: retrieval is judged by F1, not
// by the technology swap.
const CreationMaxDistance = 0.55

// CreationKnowledgeIDs is the creation tool's hybrid retrieval (05 R-47,
// creation-measure/search-f1): the vector leg within CreationMaxDistance in
// rank order, then the lexical leg's best hit admitted only when every token
// of the query is in the document (lexical.go) — what a person types when they
// know the name or one distinctive term, which the vector cutoff misses. The
// embedding's cost is returned so the session pays for it. Without an
// embedding service, or when the call fails, the lexical leg alone answers,
// flagged degraded: every-token matches first, then any-token matches.
func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string) (ids []string, costUSD float64, degraded bool, err error) {
	queries := gen.New(s.Pool)
	lexical := func(op string, limit int32) ([]string, error) {
		q := lexicalQuery(query, op)
		if q == "" {
			return nil, nil
		}
		rows, err := queries.CreationLexicalSearchSkills(ctx, gen.CreationLexicalSearchSkillsParams{Query: q, ResultLimit: limit})
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, pgconv.UUIDString(r.SkillID))
		}
		return out, nil
	}
	degradedAnswer := func() ([]string, float64, bool, error) {
		all, err := lexical("&", 3)
		if err != nil {
			return nil, 0, true, err
		}
		if len(all) < 3 {
			any, err := lexical("|", 3)
			if err != nil {
				return nil, 0, true, err
			}
			for _, id := range any {
				if !containsID(all, id) && len(all) < 3 {
					all = append(all, id)
				}
			}
		}
		return all, 0, true, nil
	}
	if s.LLM == nil {
		return degradedAnswer()
	}
	embedCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	embedResp, err := s.LLM.EmbedWithin(embedCtx, []string{query}, 10)
	if err != nil || len(embedResp.Embeddings) == 0 {
		return degradedAnswer()
	}
	if embedResp.Usage != nil && embedResp.Usage.CostUSD != nil {
		costUSD = *embedResp.Usage.CostUSD
	}
	embedding := pgvector.NewVector(embedResp.Embeddings[0])
	rows, _, err := s.hybridSearch(ctx, queries, query, &embedding, 10, searchFilters{})
	if err != nil {
		return nil, costUSD, false, err
	}
	for _, r := range rows {
		// rank is 1 - distance; an unranked row came through the lexical leg
		// with no embedding and was never measured against the query.
		if r.Rank == nil || 1-*r.Rank > CreationMaxDistance {
			continue
		}
		ids = append(ids, r.SkillID)
	}
	// The lexical admission: one document that carries every token of the
	// query, after the vector hits (measured: F1 0.877 over golden + name +
	// term queries, golden's own 0.774 and 12/12 distractor rejections kept).
	covered, err := lexical("&", 1)
	if err != nil {
		return nil, costUSD, false, err
	}
	for _, id := range covered {
		if !containsID(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, costUSD, false, nil
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
