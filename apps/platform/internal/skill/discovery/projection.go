package catalog

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type PendingEnrichment struct {
	SkillID          pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
}

func (s *Service) PendingEnrichments(ctx context.Context, limit int32) ([]PendingEnrichment, error) {
	rows, err := gen.New(s.Pool).ListPendingEnrichment(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]PendingEnrichment, 0, len(rows))
	for _, row := range rows {
		if row.PackageObjectKey == nil {
			continue
		}
		result = append(result, PendingEnrichment{
			SkillID:          row.SkillID,
			WorkspaceID:      row.WorkspaceID,
			Name:             row.Name,
			PackageObjectKey: *row.PackageObjectKey,
		})
	}
	return result, nil
}

type SkillProjection struct {
	SkillID     pgtype.UUID
	WorkspaceID pgtype.UUID
	Name        string
	Summary     string
}

type EnrichedSkillProjection struct {
	SkillID                 pgtype.UUID
	WorkspaceID             pgtype.UUID
	Name                    string
	Summary                 string
	EnrichedSummary         string
	TaskExamples            string
	Tags                    []byte
	Limitations             string
	Scan                    []byte
	Embedding               *pgvector.Vector
	EnrichmentStatus        string
	EnrichmentModel         *string
	EnrichmentPromptVersion *string
}

func IndexSkill(ctx context.Context, tx pgx.Tx, projection SkillProjection) error {
	return indexLive(ctx, tx, projection.SkillID, func(q *gen.Queries) error {
		return q.UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
			SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
			Name: projection.Name, Summary: projection.Summary,
			BigramText: LexicalIndexText(projection.Name, projection.Summary),
		})
	})
}

func IndexSkillEnriched(ctx context.Context, tx pgx.Tx, projection EnrichedSkillProjection) error {
	return indexLive(ctx, tx, projection.SkillID, func(q *gen.Queries) error {
		return q.UpsertSearchDocumentEnriched(ctx, gen.UpsertSearchDocumentEnrichedParams{
			SkillID: projection.SkillID, WorkspaceID: projection.WorkspaceID,
			Name: projection.Name, Summary: projection.Summary,
			EnrichedSummary: projection.EnrichedSummary, TaskExamples: projection.TaskExamples,
			Tags: projection.Tags, Limitations: projection.Limitations, Scan: projection.Scan,
			Embedding: projection.Embedding, EnrichmentStatus: projection.EnrichmentStatus,
			EnrichmentModel:         projection.EnrichmentModel,
			EnrichmentPromptVersion: projection.EnrichmentPromptVersion,
			BigramText:              LexicalIndexText(projection.Name, projection.Summary, projection.EnrichedSummary, projection.TaskExamples, jsonStrings(projection.Tags)),
		})
	})
}

func RefreshListing(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) error {
	return indexLive(ctx, db, skillID, func(*gen.Queries) error { return nil })
}

func indexLive(ctx context.Context, db gen.DBTX, skillID pgtype.UUID, upsert func(*gen.Queries) error) error {
	facts, live, err := registry.LiveListingFacts(ctx, db, skillID)
	if err != nil || !live {
		return err
	}
	q := gen.New(db)
	if err := upsert(q); err != nil {
		return err
	}
	return q.SetSearchDocumentListing(ctx, listingOf(skillID, facts))
}

func listingOf(skillID pgtype.UUID, facts gen.GetLiveSkillListingFactsRow) gen.SetSearchDocumentListingParams {
	listing := gen.SetSearchDocumentListingParams{
		SkillID:         skillID,
		Generated:       facts.Redistribution == string(RedistributionGenerated),
		Category:        facts.Category,
		CategorySource:  facts.CategorySource,
		LatestVersionID: facts.LatestVersionID,
		VerifiedAt:      facts.VerifiedAt,
		AgentMeasuredAt: facts.AgentMeasuredAt,
	}
	if facts.LatestVersionID.Valid {
		listing.LatestPackageObjectKey = &facts.LatestPackageObjectKey
	}
	if facts.CurationTier == string(TierCurated) {
		listing.CuratedVersionID = facts.CuratedVersionID
	}
	if facts.AgentMeasuredAt.Valid {
		listing.AgentCapability = &facts.AgentCapability
		listing.AgentRuntime = &facts.AgentRuntime
		listing.AgentRuntimeImage = &facts.AgentRuntimeImage
	}
	return listing
}

func RebuildIndex(ctx context.Context, pool *pgxpool.Pool) (indexed, pruned int64, err error) {
	skills, err := registry.LiveSkills(ctx, pool)
	if err != nil {
		return 0, 0, err
	}
	var all gen.ReindexAllParams
	for _, sk := range skills {
		all.SkillIds = append(all.SkillIds, sk.ID)
		all.WorkspaceIds = append(all.WorkspaceIds, sk.WorkspaceID)
		all.Names = append(all.Names, sk.Name)
		all.Summaries = append(all.Summaries, sk.Summary)
		all.Generated = append(all.Generated, sk.Redistribution == string(RedistributionGenerated))
	}
	q := gen.New(pool)
	if indexed, err = q.ReindexAll(ctx, all); err != nil {
		return 0, 0, err
	}
	if pruned, err = pruneRetired(ctx, q, pool); err != nil {
		return indexed, 0, err
	}
	for _, sk := range skills {
		if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return RefreshListing(ctx, tx, sk.ID) }); err != nil {
			return indexed, pruned, err
		}
	}
	return indexed, pruned, nil
}

func pruneRetired(ctx context.Context, q *gen.Queries, db gen.DBTX) (int64, error) {
	indexed, err := q.ListSearchDocumentSkillIDs(ctx)
	if err != nil {
		return 0, err
	}
	live, err := registry.LiveSkillIDs(ctx, db, indexed)
	if err != nil {
		return 0, err
	}
	alive := make(map[pgtype.UUID]bool, len(live))
	for _, id := range live {
		alive[id] = true
	}
	var retired []pgtype.UUID
	for _, id := range indexed {
		if !alive[id] {
			retired = append(retired, id)
		}
	}
	return q.PruneDeletedSearchDocuments(ctx, retired)
}

func BackfillBigram(ctx context.Context, db gen.DBTX, batch int32) (int, error) {
	q := gen.New(db)
	done := 0
	for {
		rows, err := q.ListSearchDocumentsMissingBigram(ctx, batch)
		if err != nil {
			return done, err
		}
		if len(rows) == 0 {
			return done, nil
		}
		for _, r := range rows {
			if err := q.SetSearchDocumentBigram(ctx, gen.SetSearchDocumentBigramParams{
				SkillID:    r.SkillID,
				BigramText: LexicalIndexText(r.Name, r.Summary, r.EnrichedSummary, r.TaskExamples, jsonStrings(r.Tags)),
			}); err != nil {
				return done, err
			}
			done++
		}
	}
}

func RemoveSkillFromIndex(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) error {
	return gen.New(tx).DeleteSearchDocument(ctx, gen.DeleteSearchDocumentParams{
		SkillID: skillID, WorkspaceID: workspaceID,
	})
}

func (s *Service) SkillRisks(
	ctx context.Context, workspaceID pgtype.UUID, skillIDs []pgtype.UUID,
) (map[string]json.RawMessage, error) {
	unknown, err := json.Marshal(riskHint(nil))
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(skillIDs))
	for _, id := range skillIDs {
		out[pgconv.UUIDString(id)] = unknown
	}
	if len(skillIDs) == 0 {
		return out, nil
	}

	rows, err := gen.New(s.Pool).ListSkillScans(ctx, gen.ListSkillScansParams{
		WorkspaceID: workspaceID, SkillIds: skillIDs,
	})
	if err != nil {
		return nil, err
	}
	return fillScans(out, rows, func(r gen.ListSkillScansRow) (pgtype.UUID, []byte) {
		return r.SkillID, r.Scan
	})
}

func (s *Service) CatalogSkillRisks(
	ctx context.Context, skillIDs []pgtype.UUID,
) (map[string]json.RawMessage, error) {
	unknown, err := json.Marshal(riskHint(nil))
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(skillIDs))
	for _, id := range skillIDs {
		out[pgconv.UUIDString(id)] = unknown
	}
	if len(skillIDs) == 0 {
		return out, nil
	}

	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(s.Pool).ListCatalogSkillScans(ctx, gen.ListCatalogSkillScansParams{
		SkillIds: skillIDs, CatalogWorkspaceIds: catalogs,
	})
	if err != nil {
		return nil, err
	}
	return fillScans(out, rows, func(r gen.ListCatalogSkillScansRow) (pgtype.UUID, []byte) {
		return r.SkillID, r.Scan
	})
}

func fillScans[R any](
	out map[string]json.RawMessage, rows []R, split func(R) (pgtype.UUID, []byte),
) (map[string]json.RawMessage, error) {
	for _, row := range rows {
		id, scan := split(row)
		if len(scan) == 0 {
			continue
		}
		blob, err := json.Marshal(riskHint(scan))
		if err != nil {
			return nil, err
		}
		out[pgconv.UUIDString(id)] = blob
	}
	return out, nil
}

func jsonStrings(raw []byte) string {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return ""
	}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			out = append(out, t)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return strings.Join(out, "\n")
}
