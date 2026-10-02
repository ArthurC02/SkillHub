package catalog

import (
	"context"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"log/slog"
	"sync"
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

	ReadSourceSiblings func(ctx context.Context, workspaceID pgtype.UUID, packageObjectKey string, excludedSkillID pgtype.UUID) ([]SourceSiblingFacts, error)

	CatalogWorkspaces func(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error)
	ExposedSkills     func(context.Context) ([]ExposedSkill, error)

	LLM            Model
	IntentAnalyzer IntentAnalyzer

	Budgets *modelbudget.Service

	Credit CostRecorder

	Store ObjectStore

	Analytics *analytics.Service

	packageReports queryCache[skillpkg.Report]
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

	PluginName       *string
	PluginVersion    *string
	PluginRepository *string
}

type SourceSiblingFacts struct {
	SkillID    pgtype.UUID
	Name       string
	SourcePath string
}

type searchOutcome struct {
	Interpretation SearchInterpretation
	Hits           []searchResult
	DegradedReason string
	FilteredOut    bool

	Truncated bool

	Total int64
}

type searchPurpose string

const (
	searchByPerson     searchPurpose = ""
	searchForReference searchPurpose = "reference"
)

func (s *Service) Search(ctx context.Context, query string, limit int32, filters searchFilters, purpose searchPurpose) (searchOutcome, error) {
	embedOriginal := s.embedAhead(ctx, query)
	interpretation := s.interpret(ctx, query, filters, purpose)
	return s.searchInterpreted(ctx, interpretedSearch{
		original: query, limit: limit, interpretation: interpretation, purpose: purpose, embedOriginal: embedOriginal,
	})
}

type interpretedSearch struct {
	original       string
	limit          int32
	interpretation SearchInterpretation
	purpose        searchPurpose
	embedOriginal  embedding
}

type embedding func() (*pgvector.Vector, error)

func (s *Service) embedAhead(ctx context.Context, query string) embedding {
	if s.LLM == nil {
		return nil
	}
	type answer struct {
		vec *pgvector.Vector
		err error
	}
	ready := make(chan answer, 1)
	go func() {
		vec, err := s.embedQuery(ctx, query)
		ready <- answer{vec, err}
	}()
	return sync.OnceValues(func() (*pgvector.Vector, error) {
		a := <-ready
		return a.vec, a.err
	})
}

func (s *Service) searchInterpreted(ctx context.Context, req interpretedSearch) (searchOutcome, error) {
	original, limit, interpretation, purpose := req.original, req.limit, req.interpretation, req.purpose
	filters, err := parseFilterValues(interpretation.Filters)
	if err != nil {
		return searchOutcome{}, err
	}
	query := interpretation.retrievalQuery(original)
	semanticQuery := original
	if interpretation.Status == interpretationCorrected {
		semanticQuery = query
	}
	queries := gen.New(s.Pool)

	var out searchOutcome
	out.Interpretation = interpretation

	searchStart := time.Now()
	searchMode := "hybrid"
	defer func() {
		metrics.ObserveSince(metrics.SearchDuration.WithLabelValues(searchMode), searchStart)
	}()

	queryVector := s.searchHybridFirst(ctx, queries, s.embeddingOf(ctx, semanticQuery, req), hybridRequest{
		query: semanticQuery, keywords: query, limit: limit + 1, filters: filters, maxDistance: MaxCosineDistance,
	}, &out)

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
		filteredOut, err := s.matchesWithoutFilters(ctx, queries, hybridRequest{
			query: semanticQuery, keywords: query, embedding: queryVector, limit: limit, maxDistance: MaxCosineDistance,
		})
		if err != nil {
			slog.Error("unfiltered search probe failed", "error", err)
			return searchOutcome{}, err
		}
		out.FilteredOut = filteredOut
	}

	var reasons []MatchReason
	reasonQuery := original
	if interpretation.Status == interpretationCorrected {
		reasonQuery = query
	}
	if len(out.Hits) > 0 && s.LLM != nil && purpose != searchForReference {
		reasons = s.matchReasons(ctx, reasonQuery, out.Hits)
	}
	applyMatchReasons(out.Hits, reasonQuery, reasons)

	if purpose != searchForReference {
		s.Analytics.SearchPerformed(ctx, original, len(out.Hits), filters.active())
	}
	return out, nil
}

func (s *Service) embeddingOf(ctx context.Context, semanticQuery string, req interpretedSearch) embedding {
	if req.embedOriginal != nil && semanticQuery == req.original {
		return req.embedOriginal
	}
	return func() (*pgvector.Vector, error) { return s.embedQuery(ctx, semanticQuery) }
}

func (s *Service) searchHybridFirst(ctx context.Context, queries *gen.Queries, embed embedding, req hybridRequest, out *searchOutcome) *pgvector.Vector {
	if s.LLM == nil {
		out.DegradedReason = "embedding service not configured; lexical search only"
		return nil
	}
	vec, err := embed()
	if err != nil {
		slog.Warn("query embedding failed, falling back to FTS", "error", err)
		out.DegradedReason = "embedding unavailable; lexical search only"
		return nil
	}
	req.embedding = vec
	hybridHits, total, err := s.hybridSearch(ctx, queries, req)
	if err != nil {
		slog.Warn("hybrid search failed, falling back to FTS", "error", err)
		out.DegradedReason = "hybrid search unavailable; lexical search only"
		return nil
	}
	out.Hits = hybridHits
	out.Total = total
	return vec
}

func (s *Service) matchesWithoutFilters(ctx context.Context, queries *gen.Queries, req hybridRequest) (bool, error) {
	var (
		unfiltered []searchResult
		err        error
	)
	if req.embedding != nil {
		unfiltered, _, err = s.hybridSearch(ctx, queries, req)
	} else {
		unfiltered, _, err = s.ftsOnlySearch(ctx, queries, req.keywords, req.limit, searchFilters{})
	}
	if err != nil {
		return false, err
	}
	return len(unfiltered) > 0, nil
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

type hybridRequest struct {
	query       string
	keywords    string
	embedding   *pgvector.Vector
	limit       int32
	filters     searchFilters
	maxDistance float64
}

func (s *Service) hybridSearch(ctx context.Context, queries *gen.Queries, req hybridRequest) ([]searchResult, int64, error) {
	query, keywords, embedding, limit, filters, maxDistance := req.query, req.keywords, req.embedding, req.limit, req.filters, req.maxDistance
	scope, err := s.publicScope(ctx)
	if err != nil {
		return nil, 0, err
	}
	candidates, err := queries.ListHybridSearchCandidates(ctx, gen.ListHybridSearchCandidatesParams{
		CatalogWorkspaceIds: scope.catalogs,
		ExposedKeys:         scope.exposedKeys,
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
		CatalogWorkspaceIds: scope.catalogs,
		ExposedKeys:         scope.exposedKeys,
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
		resultFacets(&hit, facetColumns{
			curated: row.Curated, category: row.Category, categorySource: row.CategorySource,
			tagsJSON: row.Tags, scanJSON: row.Scan, verifiedAt: row.VerifiedAt,
			compat: measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt),
		})
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) Browse(ctx context.Context, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	scope, err := s.publicScope(ctx)
	if err != nil {
		return nil, 0, err
	}
	queries := gen.New(s.Pool)
	rows, err := queries.BrowseCatalogSkills(ctx, gen.BrowseCatalogSkillsParams{
		CatalogWorkspaceIds: scope.catalogs,
		ExposedKeys:         scope.exposedKeys,
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
		resultFacets(&hit, facetColumns{
			curated: row.Curated, category: row.Category, categorySource: row.CategorySource,
			tagsJSON: row.Tags, scanJSON: row.Scan, verifiedAt: row.VerifiedAt,
			compat: measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt),
		})
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) ftsOnlySearch(ctx context.Context, queries *gen.Queries, query string, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	scope, err := s.publicScope(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.PublicSearchSkills(ctx, gen.PublicSearchSkillsParams{
		CatalogWorkspaceIds: scope.catalogs,
		ExposedKeys:         scope.exposedKeys,
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
		resultFacets(&hit, facetColumns{
			curated: row.Curated, category: row.Category, categorySource: row.CategorySource,
			tagsJSON: row.Tags, scanJSON: row.Scan, verifiedAt: row.VerifiedAt,
			compat: measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt),
		})
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
	if !resp.Cached {
		s.recordCallCost(ctx, credit.KindMatchReasons, resp.Model, resp.Usage)
	}
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
