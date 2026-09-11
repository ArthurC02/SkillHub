package catalog

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

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

const CreationDuplicateDistance = 0.55

const CreationMaxDistance = MaxCosineDistance

func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string, maxDistance float64) (ids []string, costUSD float64, degraded bool, err error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	queries := gen.New(s.Pool)
	lexical := func(op string, limit int32) ([]string, error) {
		q := lexicalQuery(query, op)
		if q == "" {
			return nil, nil
		}
		rows, err := queries.CreationLexicalSearchSkills(ctx, gen.CreationLexicalSearchSkillsParams{CatalogWorkspaceIds: catalogs, Query: q, ResultLimit: limit})
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

		if r.unranked {
			continue
		}
		ids = append(ids, r.SkillID)
	}
	return ids, costUSD, false, nil
}

func (s *Service) CatalogReferenceFacts(ctx context.Context, skillID, versionID string) (tier, scanStatus string, warnings int, err error) {
	var sid, vid pgtype.UUID
	if err := sid.Scan(skillID); err != nil {
		return "unknown", "unknown", 0, err
	}
	if err := vid.Scan(versionID); err != nil {
		return "unknown", "unknown", 0, err
	}
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return "unknown", "unknown", 0, err
	}
	row, err := gen.New(s.Pool).GetCatalogReferenceFacts(ctx, gen.GetCatalogReferenceFactsParams{
		SkillID: sid, VersionID: vid, CatalogWorkspaceIds: catalogs,
	})
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
