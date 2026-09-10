package catalog

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
)

type Service struct {
	Pool *pgxpool.Pool

	ReadCatalogSkill         func(context.Context, pgtype.UUID) (SkillFacts, bool, error)
	ReadWorkspaceSkill       func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadLatestVersion        func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	ReadRuntimeCompatibility func(context.Context, pgtype.UUID) (RuntimeCompatibilityFacts, bool, error)

	SourceByID func(context.Context, pgtype.UUID, pgtype.UUID) (SourceFacts, bool, error)

	LLM *llmclient.Client

	Credit CostRecorder

	Store ObjectStore

	Analytics *analytics.Service
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
	Hits           []searchResult
	DegradedReason string
	FilteredOut    bool

	Truncated bool

	Total int64
}

func (s *Service) Search(ctx context.Context, query string, limit int32, filters searchFilters, silent bool) (searchOutcome, error) {
	queries := gen.New(s.Pool)

	var (
		out searchOutcome

		embedding *pgvector.Vector
	)

	searchStart := time.Now()
	searchMode := "hybrid"
	defer func() {
		metrics.ObserveSince(metrics.SearchDuration.WithLabelValues(searchMode), searchStart)
	}()

	if s.LLM == nil {
		out.DegradedReason = "embedding service not configured; lexical search only"
	} else if vec, err := s.embedQuery(ctx, query); err != nil {
		slog.Warn("query embedding failed, falling back to FTS", "error", err)
		out.DegradedReason = "embedding unavailable; lexical search only"
	} else if hybridHits, total, err := s.hybridSearch(ctx, queries, query, vec, limit+1, filters, MaxCosineDistance); err != nil {
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
			unfiltered, _, err = s.hybridSearch(ctx, queries, query, embedding, limit, searchFilters{}, MaxCosineDistance)
		} else {
			unfiltered, _, err = s.ftsOnlySearch(ctx, queries, query, limit, searchFilters{})
		}
		if err != nil {
			slog.Error("unfiltered search probe failed", "error", err)
			return searchOutcome{}, err
		}
		out.FilteredOut = len(unfiltered) > 0
	}

	var reasons []llmclient.MatchReason
	if len(out.Hits) > 0 && s.LLM != nil && !silent {
		reasons = s.matchReasons(ctx, query, out.Hits)
	}
	applyMatchReasons(out.Hits, query, reasons)

	if !silent {
		s.Analytics.SearchPerformed(ctx, query, len(out.Hits), filters.active())
	}
	return out, nil
}

func (s *Service) embedQuery(ctx context.Context, query string) (*pgvector.Vector, error) {

	// budget-over: app.EMBED_TIMEOUT_SECONDS
	embedCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	embedResp, err := s.LLM.EmbedWithin(embedCtx, []string{query}, 10)
	if err != nil {
		return nil, err
	}

	s.recordSearchCost(ctx, embedResp)
	if len(embedResp.Embeddings) == 0 {

		return nil, errors.New("catalog: embed returned no vectors")
	}
	embedding := pgvector.NewVector(embedResp.Embeddings[0])
	return &embedding, nil
}

func (s *Service) hybridSearch(ctx context.Context, queries *gen.Queries, query string, embedding *pgvector.Vector, limit int32, filters searchFilters, maxDistance float64) ([]searchResult, int64, error) {
	rows, err := queries.PublicHybridSearchSkills(ctx, gen.PublicHybridSearchSkillsParams{
		Query:          query,
		BigramQuery:    lexicalQuery(query, "&"),
		QueryEmbedding: embedding,
		MaxDistance:    maxDistance,
		ResultLimit:    limit,
		HasScript:      filters.HasScript,
		SpecValidated:  filters.SpecValidated,
		AgentRuntime:   filters.AgentRuntime,
		CurationTier:   filters.CurationTier,
		Category:       filters.Category,
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
			Summary:       row.Summary,
			SummarySource: row.SummarySource,
			unranked:      row.Unranked,
		}

		if !row.Unranked {
			rank := row.Rank
			hit.Rank = &rank
		} else {
			hit.RankNote = rankNotePendingItem
		}
		resultFacets(&hit, row.CurationTier, row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) Browse(ctx context.Context, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	queries := gen.New(s.Pool)
	rows, err := queries.BrowseCatalogSkills(ctx, gen.BrowseCatalogSkillsParams{
		ResultLimit:   limit,
		HasScript:     filters.HasScript,
		SpecValidated: filters.SpecValidated,
		AgentRuntime:  filters.AgentRuntime,
		CurationTier:  filters.CurationTier,
		Category:      filters.Category,
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
			Summary:       row.Summary,
			SummarySource: row.SummarySource,

			RankNote: rankNoteCatalog,
		}
		resultFacets(&hit, row.CurationTier, row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) ftsOnlySearch(ctx context.Context, queries *gen.Queries, query string, limit int32, filters searchFilters) ([]searchResult, int64, error) {
	rows, err := queries.PublicSearchSkills(ctx, gen.PublicSearchSkillsParams{
		Query:         query,
		BigramQuery:   lexicalQuery(query, "&"),
		ResultLimit:   limit,
		HasScript:     filters.HasScript,
		SpecValidated: filters.SpecValidated,
		AgentRuntime:  filters.AgentRuntime,
		CurationTier:  filters.CurationTier,
		Category:      filters.Category,
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
			Summary:       row.Summary,
			SummarySource: row.SummarySource,
			RankNote:      rankNoteDegraded,
		}
		resultFacets(&hit, row.CurationTier, row.Category, row.CategorySource, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, total, nil
}

func (s *Service) matchReasons(ctx context.Context, query string, hits []searchResult) []llmclient.MatchReason {
	n := min(len(hits), 10)
	candidates := make([]llmclient.SkillCandidate, n)
	for i := 0; i < n; i++ {
		candidates[i] = llmclient.SkillCandidate{
			SkillID: hits[i].SkillID,
			Name:    hits[i].Name,
			Summary: hits[i].Summary,
		}
	}

	// budget-over: app.MATCH_REASONS_TIMEOUT_SECONDS
	reasonCtx, cancel := context.WithTimeout(ctx, 13*time.Second)
	defer cancel()

	resp, err := s.LLM.MatchReasons(reasonCtx, query, candidates)
	if err != nil {
		slog.Warn("match-reasons call failed, using template fallback", "error", err)
		return nil
	}
	return resp.Reasons
}

func (s *Service) SearchWorkspace(ctx context.Context, workspaceID pgtype.UUID, query string, limit int32) ([]gen.SearchSkillsRow, error) {
	return gen.New(s.Pool).SearchSkills(ctx, gen.SearchSkillsParams{
		WorkspaceID: workspaceID,
		Query:       query,
		Limit:       limit,
	})
}
