package catalog

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

// budget-over: app.MATCH_REASONS_TIMEOUT_SECONDS
const matchReasonsDeadline = 13 * time.Second

// MatchReasonsBudget names this call for an operator and bounds what they may set.
var MatchReasonsBudget = modelbudget.Endpoint{Kind: "match-reasons", Deadline: matchReasonsDeadline}

type Service struct {
	Pool *pgxpool.Pool

	ReadLiveListingFacts func(context.Context, gen.DBTX, pgtype.UUID) (ListingFacts, bool, error)
	ReadLiveSkills       func(context.Context, gen.DBTX) ([]IndexSkillFacts, error)
	ReadLiveSkillIDs     func(context.Context, gen.DBTX, []pgtype.UUID) ([]pgtype.UUID, error)

	ReadCatalogSkill         func(context.Context, pgtype.UUID) (SkillFacts, bool, error)
	ReadWorkspaceSkill       func(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error)
	ReadLatestVersion        func(ctx context.Context, workspaceID, skillID pgtype.UUID) (VersionFacts, bool, error)
	ReadRuntimeCompatibility func(context.Context, pgtype.UUID) (RuntimeCompatibilityFacts, bool, error)

	SourceByID func(ctx context.Context, workspaceID, sourceID pgtype.UUID) (SourceFacts, bool, error)

	CatalogWorkspaces func(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error)

	LLM            Model
	IntentAnalyzer IntentAnalyzer

	Budgets *modelbudget.Service

	Credit CostRecorder

	Store ObjectStore

	Analytics *analytics.Service
}

type ListingFacts struct {
	Redistribution         string
	Category               *string
	CategorySource         *string
	CurationTier           string
	CuratedVersionID       pgtype.UUID
	LatestVersionID        pgtype.UUID
	VerifiedAt             pgtype.Timestamptz
	LatestPackageObjectKey string
	LatestSourcePath       string
	AgentCapability        string
	AgentRuntime           string
	AgentRuntimeImage      string
	AgentMeasuredAt        pgtype.Timestamptz
}

type IndexSkillFacts struct {
	ID             pgtype.UUID
	WorkspaceID    pgtype.UUID
	Name           string
	Summary        string
	Redistribution string
}

type SkillFacts struct {
	ID                  pgtype.UUID
	WorkspaceID         pgtype.UUID
	Name                string
	Summary             *string
	ForkedFromSkillID   pgtype.UUID
	ForkedFromVersionID pgtype.UUID
	TakedownAt          pgtype.Timestamptz
	AccessRestriction   *string
	Redistribution      string
	CurationTier        string
	CuratedVersionID    pgtype.UUID
	Category            *string

	CategorySource *string
}

type VersionFacts struct {
	ID                pgtype.UUID
	WorkspaceID       pgtype.UUID
	SourceID          pgtype.UUID
	VersionNumber     int32
	ContentHash       string
	PackageObjectKey  string
	SourcePath        string
	LicenseExpression *string
	CreatedAt         pgtype.Timestamptz
	LicenseSource     *string
}

type RuntimeCompatibilityFacts struct {
	Capability   string
	Runtime      string
	RuntimeImage string
	MeasuredAt   pgtype.Timestamptz
}

type SourceFacts struct {
	SourceType       string
	SourceURL        *string
	SourceRef        *string
	ContentHash      string
	FetchedAt        pgtype.Timestamptz
	LastCheckedAt    pgtype.Timestamptz
	UnavailableSince pgtype.Timestamptz

	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string

	GenerationInputs []byte
}

type searchOutcome struct {
	Interpretation SearchInterpretation
	Hits           []searchResult
	DegradedReason string
	FilteredOut    bool

	Truncated bool

	Total int64
}

func (s *Service) Search(ctx context.Context, query string, limit int32, filters searchFilters, silent bool) (searchOutcome, error) {
	interpretation := s.interpret(ctx, query, filters, silent)
	return s.searchInterpreted(ctx, query, limit, interpretation, silent)
}

func (s *Service) searchInterpreted(ctx context.Context, original string, limit int32, interpretation SearchInterpretation, silent bool) (searchOutcome, error) {
	filters, err := parseFilterValues(interpretation.Filters)
	if err != nil {
		return searchOutcome{}, err
	}
	query := interpretation.retrievalQuery(original)
	semanticQuery := original
	if interpretation.Status == "corrected" {
		semanticQuery = query
	}
	queries := gen.New(s.Pool)

	var (
		out searchOutcome

		embedding *pgvector.Vector
	)
	out.Interpretation = interpretation

	searchStart := time.Now()
	searchMode := "hybrid"
	defer func() {
		metrics.ObserveSince(metrics.SearchDuration.WithLabelValues(searchMode), searchStart)
	}()

	if s.LLM == nil {
		out.DegradedReason = "embedding service not configured; lexical search only"
	} else if vec, err := s.embedQuery(ctx, semanticQuery); err != nil {
		slog.Warn("query embedding failed, falling back to FTS", "error", err)
		out.DegradedReason = "embedding unavailable; lexical search only"
	} else if hybridHits, total, err := s.hybridSearchWithKeywords(ctx, queries, semanticQuery, query, vec, limit+1, filters, MaxCosineDistance); err != nil {
		slog.Warn("hybrid search failed, falling back to FTS", "error", err)
		out.DegradedReason = "hybrid search unavailable; lexical search only"
	} else {
		embedding = vec
		out.Hits = hybridHits
		out.Total = total
	}

	if out.DegradedReason != "" {
		searchMode = "fts"
		hits, total, err := s.ftsOnlySearch(ctx, queries, query, limit+1, filters)
		if err != nil {
			slog.Error("lexical search failed", "error", err)
			return searchOutcome{}, err
		}
		out.Hits = hits

		out.Total = total
	}

	if out.Hits == nil {
		out.Hits = []searchResult{}
	}

	if len(out.Hits) > int(limit) {
		out.Hits = out.Hits[:limit]
		out.Truncated = true
	}

	if len(out.Hits) == 0 && filters.active() {
		var (
			unfiltered []searchResult
			err        error
		)

		if embedding != nil {
			unfiltered, _, err = s.hybridSearchWithKeywords(ctx, queries, semanticQuery, query, embedding, limit, searchFilters{}, MaxCosineDistance)
		} else {
			unfiltered, _, err = s.ftsOnlySearch(ctx, queries, query, limit, searchFilters{})
		}
		if err != nil {
			slog.Error("unfiltered search probe failed", "error", err)
			return searchOutcome{}, err
		}
		out.FilteredOut = len(unfiltered) > 0
	}

	var reasons []MatchReason
	reasonQuery := original
	if interpretation.Status == "corrected" {
		reasonQuery = query
	}
	if len(out.Hits) > 0 && s.LLM != nil && !silent {
		reasons = s.matchReasons(ctx, reasonQuery, out.Hits)
	}
	applyMatchReasons(out.Hits, reasonQuery, reasons)

	if !silent {
		s.Analytics.SearchPerformed(ctx, original, len(out.Hits), filters.active())
	}
	return out, nil
}

func (s *Service) embedQuery(ctx context.Context, query string) (*pgvector.Vector, error) {

	// budget-over: app.EMBED_TIMEOUT_SECONDS
	embedCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	embedResp, err := s.LLM.Embed(embedCtx, []string{query}, 10*time.Second)
	if err != nil {
		return nil, err
	}

	s.recordSearchCost(ctx, embedResp)
	if len(embedResp.Vectors) == 0 {

		return nil, errors.New("catalog: embed returned no vectors")
	}
	embedding := pgvector.NewVector(embedResp.Vectors[0])
	return &embedding, nil
}

func (s *Service) catalogWorkspaceIDs(ctx context.Context) ([]pgtype.UUID, error) {
	if s.CatalogWorkspaces == nil {
		return nil, errors.New("catalog: catalog workspace read not injected")
	}
	return s.CatalogWorkspaces(ctx, s.Pool)
}

const (
	vectorCandidates   = int32(50)
	fulltextCandidates = int32(50)
	lexicalCandidates  = int32(5)
)

func (s *Service) hybridSearch(ctx context.Context, queries *gen.Queries, query string, embedding *pgvector.Vector, limit int32, filters searchFilters, maxDistance float64) ([]searchResult, int64, error) {
	return s.hybridSearchWithKeywords(ctx, queries, query, query, embedding, limit, filters, maxDistance)
}

func (s *Service) hybridSearchWithKeywords(ctx context.Context, queries *gen.Queries, query, keywords string, embedding *pgvector.Vector, limit int32, filters searchFilters, maxDistance float64) ([]searchResult, int64, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	candidates, err := queries.ListHybridSearchCandidates(ctx, gen.ListHybridSearchCandidatesParams{
		CatalogWorkspaceIds: catalogs,
		Query:               keywords,
		BigramQuery:         lexicalQuery(query, "&"),
		QueryEmbedding:      embedding,
		VectorCandidates:    vectorCandidates,
		FulltextCandidates:  fulltextCandidates,
		LexicalCandidates:   lexicalCandidates,
	})
	if err != nil {
		return nil, 0, err
	}
	fused, order := fuseHybridCandidates(candidates)
	admitted := admittedCandidates(fused, order, maxDistance)
	if len(admitted) == 0 {
		return []searchResult{}, 0, nil
	}
	rows, err := queries.ListHybridSearchDocuments(ctx, gen.ListHybridSearchDocumentsParams{
		SkillIds:            admitted,
		CatalogWorkspaceIds: catalogs,
		HasScript:           filters.HasScript,
		SpecValidated:       filters.SpecValidated,
		AgentRuntime:        filters.AgentRuntime,
		Curated:             curatedFilter(filters.CurationTier),
		Category:            filters.Category,
	})
	if err != nil {
		return nil, 0, err
	}
	rankHybridDocuments(rows, fused, query)
	rows, total := firstPage(rows, limit)

	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		candidate := fused[row.SkillID]
		hit := searchResult{
			SkillID:       pgconv.UUIDString(row.SkillID),
			Name:          row.Name,
			Summary:       summaryText(row.Summary, row.EnrichedSummary),
			SummarySource: summarySource(row.EnrichedSummary),
			unranked:      !candidate.ranked,
		}

		if candidate.ranked {
			rank := 1 - candidate.distance
			hit.Rank = &rank
		} else {
			hit.RankNote = rankNotePendingItem
		}
		resultFacets(&hit, tierOf(row.Curated), row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) Browse(ctx context.Context, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	queries := gen.New(s.Pool)
	rows, err := queries.BrowseCatalogSkills(ctx, gen.BrowseCatalogSkillsParams{
		CatalogWorkspaceIds: catalogs,
		ResultLimit:         limit,
		HasScript:           filters.HasScript,
		SpecValidated:       filters.SpecValidated,
		AgentRuntime:        filters.AgentRuntime,
		Curated:             curatedFilter(filters.CurationTier),
		Category:            filters.Category,
	})
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalMatches
	}

	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		hit := searchResult{
			SkillID:       pgconv.UUIDString(row.SkillID),
			Name:          row.Name,
			Summary:       summaryText(row.Summary, row.EnrichedSummary),
			SummarySource: summarySource(row.EnrichedSummary),

			RankNote: rankNoteCatalog,
		}
		resultFacets(&hit, tierOf(row.Curated), row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) ftsOnlySearch(ctx context.Context, queries *gen.Queries, query string, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.PublicSearchSkills(ctx, gen.PublicSearchSkillsParams{
		CatalogWorkspaceIds: catalogs,
		Query:               query,
		BigramQuery:         lexicalQuery(query, "&"),
		ResultLimit:         limit,
		HasScript:           filters.HasScript,
		SpecValidated:       filters.SpecValidated,
		AgentRuntime:        filters.AgentRuntime,
		Curated:             curatedFilter(filters.CurationTier),
		Category:            filters.Category,
	})
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalMatches
	}

	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		hit := searchResult{
			SkillID:       pgconv.UUIDString(row.SkillID),
			Name:          row.Name,
			Summary:       summaryText(row.Summary, row.EnrichedSummary),
			SummarySource: summarySource(row.EnrichedSummary),
			RankNote:      rankNoteDegraded,
		}
		resultFacets(&hit, tierOf(row.Curated), row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) matchReasons(ctx context.Context, query string, hits []searchResult) []MatchReason {
	n := min(len(hits), 10)
	candidates := make([]SkillCandidate, n)
	for i := 0; i < n; i++ {
		candidates[i] = SkillCandidate{
			SkillID: hits[i].SkillID,
			Name:    hits[i].Name,
			Summary: hits[i].Summary,
		}
	}

	reasonCtx, cancel := context.WithTimeout(ctx, matchReasonsDeadline)
	defer cancel()

	resp, err := s.LLM.MatchReasons(reasonCtx, query, candidates, s.Budgets.Within(ctx, MatchReasonsBudget))
	if err != nil {
		slog.Warn("match-reasons call failed, using template fallback", "error", err)
		return nil
	}
	s.recordCallCost(ctx, credit.KindMatchReasons, resp.Model, resp.Usage)
	return resp.Reasons
}

type WorkspaceSearchHit struct {
	SkillID string
	Name    string
	Summary string
}

func (s *Service) SearchWorkspace(ctx context.Context, workspaceID pgtype.UUID, query string, limit int32) ([]WorkspaceSearchHit, error) {
	rows, err := gen.New(s.Pool).SearchSkills(ctx, gen.SearchSkillsParams{
		WorkspaceID: workspaceID,
		Query:       query,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	hits := make([]WorkspaceSearchHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, WorkspaceSearchHit{SkillID: pgconv.UUIDString(row.SkillID), Name: row.Name, Summary: row.Summary})
	}
	return hits, nil
}
