package catalog

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/jackc/pgx/v5/pgtype"
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

// CreationDuplicateDistance is the duplicate guard's cut-off (05 R-50): "is
// there already a Skill that IS this draft" is a stricter question than "is
// there a Skill worth looking at", so it keeps the 0.55 the creation tool was
// measured at when precision at one result was the goal (creation-measure/
// search-f1, 2026-09-06: at 0.55 the vector leg answers with precision 0.92
// and rejects every distractor).
const CreationDuplicateDistance = 0.55

// CreationMaxDistance is the cut-off for the creation tool's catalogue search
// and the first-message offer (05 R-49). It is the public search's own
// MaxCosineDistance since the F1 loop of 2026-09-07 (report §15): with the v7
// enrichment the two rules were swept on golden + name + term queries under
// F1 at k = |relevant| — 0.941 at 0.55, 0.948 at 0.60, 0.955 at 0.75, with
// every distractor still rejected at every cut-off — so the creation tool
// simply runs the public rule, and there is one rule to measure.
const CreationMaxDistance = MaxCosineDistance

// CreationKnowledgeIDs is the creation tool's hybrid retrieval (05 R-47／
// R-48, creation-measure/search-f1): the public rule — covered bigram hits
// first (in their own distance order), then the vector hits within
// maxDistance, the exact name pinned — minus the rows that were never
// measured against the query (no embedding yet): an offer to a person is a
// semantic answer or nothing. The embedding's cost is returned so the session
// pays for it. Without an embedding service, or when the call fails, the
// lexical leg alone answers, flagged degraded: every-token matches first, then
// any-token matches.
func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string, maxDistance float64) (ids []string, costUSD float64, degraded bool, err error) {
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
	rows, _, err := s.hybridSearch(ctx, queries, query, &embedding, 10, searchFilters{}, maxDistance)
	if err != nil {
		return nil, costUSD, false, err
	}
	for _, r := range rows {
		// The SQL already keeps covered rows past the cut-off and orders the
		// page; only the rows never measured against the query are dropped.
		if r.unranked {
			continue
		}
		ids = append(ids, r.SkillID)
	}
	return ids, costUSD, false, nil
}

// CatalogReferenceFacts is what the creation tool shows beside a Skill it
// offers to adopt or reference (05 SEC-013, LLM04: an offer carries the same
// trust facts as a search row). Tier is curated only when the offered version
// is the curated one; scan status and warnings come from the projected import
// scan, never a guess (DISC-004). A Skill outside the catalogue answers unknown.
func (s *Service) CatalogReferenceFacts(ctx context.Context, skillID, versionID string) (tier, scanStatus string, warnings int, err error) {
	var sid, vid pgtype.UUID
	if err := sid.Scan(skillID); err != nil {
		return "unknown", "unknown", 0, err
	}
	if err := vid.Scan(versionID); err != nil {
		return "unknown", "unknown", 0, err
	}
	row, err := gen.New(s.Pool).GetCatalogReferenceFacts(ctx, gen.GetCatalogReferenceFactsParams{SkillID: sid, VersionID: vid})
	if err != nil {
		return "unknown", "unknown", 0, err
	}
	tier = "indexed"
	if row.Curated {
		tier = "curated"
	}
	risk := riskHint(row.Scan)
	return tier, risk.ScanStatus, risk.Warnings, nil
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
