package catalog

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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

// CreationKnowledgeIDs is the semantic sibling: the query is embedded (one
// embedding call on the LLM client's key — the owner chose to spend it,
// 2026-09-06 "Embedding 直接花錢測試") and ranked by the same hybrid retrieval
// the public search uses, so cross-language and paraphrased tasks find the
// Skills the lexical tool misses (ADR-013). Falls back to lexical, flagged,
// when no embedding service is wired.
func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string) ([]string, bool, error) {
	queries := gen.New(s.Pool)
	if s.LLM == nil {
		ids, err := s.CreationReferenceIDs(ctx, query)
		return ids, true, err
	}
	embedding, err := s.embedQuery(ctx, query)
	if err != nil {
		ids, ferr := s.CreationReferenceIDs(ctx, query)
		return ids, true, ferr
	}
	rows, _, err := s.hybridSearch(ctx, queries, query, embedding, 10, searchFilters{})
	if err != nil {
		return nil, false, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.SkillID)
	}
	return ids, false, nil
}
