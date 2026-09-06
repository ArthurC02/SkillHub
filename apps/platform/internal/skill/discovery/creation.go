package catalog

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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

// CreationKnowledgeIDs is the semantic sibling: the query is embedded (one
// embedding call, whose cost is returned so the session pays for it) and
// ranked by the same hybrid retrieval the public search uses, then cut at
// CreationMaxDistance. Falls back to lexical, flagged, when no embedding
// service is wired or the embedding call fails.
func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string) (ids []string, costUSD float64, degraded bool, err error) {
	queries := gen.New(s.Pool)
	if s.LLM == nil {
		ids, err = s.CreationReferenceIDs(ctx, query)
		return ids, 0, true, err
	}
	embedCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	embedResp, err := s.LLM.EmbedWithin(embedCtx, []string{query}, 10)
	if err != nil || len(embedResp.Embeddings) == 0 {
		ids, ferr := s.CreationReferenceIDs(ctx, query)
		return ids, 0, true, ferr
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
	return ids, costUSD, false, nil
}
