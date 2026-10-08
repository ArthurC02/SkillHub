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

type CreationKnowledge struct {
	IDs      []string
	CostUSD  float64
	Degraded bool
}

func (s *Service) CreationKnowledgeIDs(ctx context.Context, query string, maxDistance float64) (CreationKnowledge, error) {
	scope, err := s.publicScope(ctx)
	if err != nil {
		return CreationKnowledge{}, err
	}
	words := creationWordSearch{queries: gen.New(s.Pool), scope: scope, query: query}
	if s.LLM == nil {
		ids, err := words.degradedIDs(ctx)
		return CreationKnowledge{IDs: ids, Degraded: true}, err
	}
	embedding, costUSD, ok := s.embedCreationQuery(ctx, query)
	if !ok {
		ids, err := words.degradedIDs(ctx)
		return CreationKnowledge{IDs: ids, Degraded: true}, err
	}
	ids, err := s.rankedCreationIDs(ctx, words.queries, query, embedding, maxDistance)
	if err != nil {
		return CreationKnowledge{CostUSD: costUSD}, err
	}
	return CreationKnowledge{IDs: ids, CostUSD: costUSD}, nil
}

type creationWordSearch struct {
	queries *gen.Queries
	scope   publicScope
	query   string
}

func (w creationWordSearch) ids(ctx context.Context, op string, limit int32) ([]string, error) {
	q := lexicalQuery(w.query, op)
	if q == "" {
		return nil, nil
	}
	rows, err := w.queries.CreationLexicalSearchSkills(ctx, gen.CreationLexicalSearchSkillsParams{CatalogWorkspaceIds: w.scope.catalogs, ExposedKeys: w.scope.exposedKeys, ExposedSkillIds: w.scope.exposedSkillIDs, Query: q, ResultLimit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, pgconv.UUIDString(r.SkillID))
	}
	return out, nil
}

const degradedKnowledgeLimit = 3

func (w creationWordSearch) degradedIDs(ctx context.Context) ([]string, error) {
	all, err := w.ids(ctx, "&", degradedKnowledgeLimit)
	if err != nil {
		return nil, err
	}
	if len(all) < degradedKnowledgeLimit {
		any, err := w.ids(ctx, "|", degradedKnowledgeLimit)
		if err != nil {
			return nil, err
		}
		for _, id := range any {
			if !containsID(all, id) && len(all) < degradedKnowledgeLimit {
				all = append(all, id)
			}
		}
	}
	return all, nil
}

func (s *Service) embedCreationQuery(ctx context.Context, query string) (pgvector.Vector, float64, bool) {
	embedCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	embedResp, err := s.LLM.Embed(embedCtx, []string{query}, 10*time.Second)
	if err != nil || len(embedResp.Vectors) == 0 {
		return pgvector.Vector{}, 0, false
	}
	var costUSD float64
	if embedResp.Usage != nil && embedResp.Usage.CostUSD != nil {
		costUSD = *embedResp.Usage.CostUSD
	}
	return pgvector.NewVector(embedResp.Vectors[0]), costUSD, true
}

func (s *Service) rankedCreationIDs(ctx context.Context, queries *gen.Queries, query string, embedding pgvector.Vector, maxDistance float64) ([]string, error) {
	rows, _, err := s.hybridSearch(ctx, queries, hybridRequest{
		query: query, keywords: query, embedding: &embedding, limit: 10, maxDistance: maxDistance,
	})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range rows {
		if r.unranked {
			continue
		}
		ids = append(ids, r.SkillID)
	}
	return ids, nil
}

type ReferenceFacts struct {
	Tier       string
	ScanStatus string
	Warnings   int
}

var unknownReferenceFacts = ReferenceFacts{Tier: "unknown", ScanStatus: "unknown"}

func (s *Service) CatalogReferenceFacts(ctx context.Context, skillID, versionID string) (ReferenceFacts, error) {
	var sid, vid pgtype.UUID
	if err := sid.Scan(skillID); err != nil {
		return unknownReferenceFacts, err
	}
	if err := vid.Scan(versionID); err != nil {
		return unknownReferenceFacts, err
	}
	scope, err := s.publicScope(ctx)
	if err != nil {
		return unknownReferenceFacts, err
	}
	row, err := gen.New(s.Pool).GetCatalogReferenceFacts(ctx, gen.GetCatalogReferenceFactsParams{
		SkillID: sid, CatalogWorkspaceIds: scope.catalogs, ExposedKeys: scope.exposedKeys, ExposedSkillIds: scope.exposedSkillIDs,
	})
	if err != nil {
		return unknownReferenceFacts, err
	}
	tier := "indexed"
	if row.CuratedVersionID.Valid && row.CuratedVersionID.Bytes == vid.Bytes {
		tier = "curated"
	}
	risk := riskHint(row.Scan)
	return ReferenceFacts{Tier: tier, ScanStatus: risk.ScanStatus, Warnings: risk.Warnings}, nil
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
