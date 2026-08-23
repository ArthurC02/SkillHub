package catalog

// The application layer for discovery: retrieval, ranking, detail assembly and
// the operator hold. Everything here takes a context and structured arguments
// and returns structured results — no *http.Request, no ResponseWriter, no
// status codes. The handlers in http.go, detail.go and restriction.go parse
// requests, decide the workspace scope (iron rule 3) and map the sentinel
// errors below onto status codes.

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
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// Service owns the non-HTTP dependencies of discovery.
type Service struct {
	Pool *pgxpool.Pool
	// Registry owns Skill, Version and runtime-compatibility facts.
	Registry *registry.Service
	// SourceByID is ingest's owner read, adapted at the composition root because
	// catalog -> ingest is deliberately denied to avoid a cycle.
	SourceByID func(context.Context, pgtype.UUID, pgtype.UUID) (SourceFacts, bool, error)
	// LLM provides query embeddings and match reasons. nil = embedding
	// unavailable, FTS-only fallback.
	LLM *llmclient.Client
	// Store reads stored packages for the detail and file views. nil = those
	// views report the package scan as unavailable rather than clean.
	Store ObjectStore
	// Analytics records the two funnel events that happen here (02:O11Y-004): an
	// intent was submitted, and a detail page was opened. Nil, or one with no
	// retention period configured, records nothing — see internal/product/learning for
	// why not collecting is the correct default until PDM-006 ratifies a retention.
	//
	// Never a source of truth and never able to fail a search: every call is fire
	// and forget (ADR-029 決策 1).
	Analytics *analytics.Service
}

// SourceFacts is the provenance shape consumed by Catalog detail views.
type SourceFacts struct {
	SourceType       string
	SourceURL        *string
	SourceRef        *string
	ContentHash      string
	FetchedAt        pgtype.Timestamptz
	LastCheckedAt    pgtype.Timestamptz
	UnavailableSince pgtype.Timestamptz
	// The generation record (GEN-006), nil for every other source type.
	TaskDescription        *string
	GeneratorModel         *string
	GeneratorPromptVersion *string
}

// searchOutcome is what one retrieval run produced, with the two facts about
// *how* it was produced that the envelope has to report separately (see
// searchResponse.Degraded and .FilteredOut for why they never merge).
type searchOutcome struct {
	Hits           []searchResult
	DegradedReason string
	FilteredOut    bool
	// Truncated says the catalogue had more matches than this page shows. The cap
	// was always here — 20 by default — and hit 21 simply did not exist as far as
	// the caller could tell. ADR-042 決策 3 makes that the defect it is: a
	// truncated list has to state that it was truncated, and 「完全不設限」 stopped
	// being an available answer. Retrieval asks for one more row than it will
	// return, which is the same trick GET /skills uses and costs one row.
	Truncated bool
}

// Search runs the DISC-001/002/003 retrieval pipeline for an already-validated
// query: hybrid retrieval where possible, the lexical floor where not, the
// filtered-out probe when the filters emptied the page, and one batched
// match-reason call for the whole page.
//
// Scope is fixed to catalog workspaces inside the SQL (CORE-006) — an anonymous
// caller has no session to derive a scope from and must never supply one.
func (s *Service) Search(ctx context.Context, query string, limit int32, filters searchFilters) (searchOutcome, error) {
	queries := gen.New(s.Pool)

	// Hybrid retrieval is the intended path. Degrading to FTS-only is an
	// availability floor, not an equivalent alternative: ADR-013 定案調整 1
	// measured the vector leg carrying cross-language recall that the lexical
	// leg misses entirely, so a degraded answer is flagged rather than passed
	// off as a normal one. The reverse degradation (vector-only when FTS is
	// down) is not a case that exists: both legs live in the same database.
	var (
		out searchOutcome
		// Kept so the filtered-out probe below can re-run the same retrieval
		// without paying for a second embedding call.
		embedding *pgvector.Vector
	)

	// O11Y-001 / NFR-004: search latency, measured from here rather than from the
	// top of the handler, so a blank or incomprehensible query - which never
	// reaches retrieval - does not flatter the percentile with a zero. The two
	// legs are separate series because a degraded FTS-only answer is a different
	// product from a hybrid one and averaging them hides exactly that.
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
	} else if hybridHits, err := s.hybridSearch(ctx, queries, query, vec, limit+1, filters); err != nil {
		slog.Warn("hybrid search failed, falling back to FTS", "error", err)
		out.DegradedReason = "hybrid search unavailable; lexical search only"
	} else {
		embedding = vec
		out.Hits = hybridHits
	}

	if out.DegradedReason != "" {
		searchMode = "fts"
		hits, err := s.ftsOnlySearch(ctx, queries, query, limit+1, filters)
		if err != nil {
			slog.Error("lexical search failed", "error", err)
			return searchOutcome{}, err
		}
		out.Hits = hits
	}

	if out.Hits == nil {
		out.Hits = []searchResult{}
	}

	// Trimmed here rather than in the handler, and before the match-reason call:
	// the probe row exists only to answer "was there more", and paying a model
	// call to explain a hit nobody will see is the kind of cost that gets noticed
	// once and never explained.
	if len(out.Hits) > int(limit) {
		out.Hits = out.Hits[:limit]
		out.Truncated = true
	}

	// An empty filtered page has two very different causes, and the same
	// retrieval run without the filters is the only thing that tells them apart.
	// It costs one more SQL round trip, only in the empty case, and no model
	// call — the embedding is already in hand.
	if len(out.Hits) == 0 && filters.active() {
		var (
			unfiltered []searchResult
			err        error
		)
		if embedding != nil {
			unfiltered, err = s.hybridSearch(ctx, queries, query, embedding, limit, searchFilters{})
		} else {
			unfiltered, err = s.ftsOnlySearch(ctx, queries, query, limit, searchFilters{})
		}
		if err != nil {
			slog.Error("unfiltered search probe failed", "error", err)
			return searchOutcome{}, err
		}
		out.FilteredOut = len(unfiltered) > 0
	}

	// DISC-002: one batched call for the whole result page, never one per hit.
	var reasons []llmclient.MatchReason
	if len(out.Hits) > 0 && s.LLM != nil {
		reasons = s.matchReasons(ctx, query, out.Hits)
	}
	applyMatchReasons(out.Hits, query, reasons)

	// Funnel segment 1 (02:O11Y-004). The query's length, script, hit count and
	// whether filters were on — never the words themselves, which can carry
	// personal data and only reach BETA-003's consented channel (ADR-029 決策 2).
	//
	// The public search endpoint has no session middleware at all (DISC-010:
	// public search does not require login), so the event carries no workspace.
	// That is the honest shape rather than a gap: the first funnel segment
	// routinely happens before anyone signs in, and session_id is what stitches
	// it to the later ones.
	s.Analytics.SearchPerformed(ctx, query, len(out.Hits), filters.active())
	return out, nil
}

// embedQuery gets the query vector from the LLM service. Split out of
// hybridSearch so the retrieval SQL can be re-run (with different filters)
// without paying for the model call twice.
//
// ponytail: the raw query is embedded as typed. ADR-013 定案調整 2 puts LLM
// query rewriting ahead of the embedding call as a Top-1 precision gain (80% ->
// 100% in the spike), not as a recall requirement — its whole degradation path
// is "embed the original sentence", which is what this does unconditionally.
// Add the rewrite step once the end-to-end p95 budget (NFR-004) has been
// measured with a real gateway; adding it before that measurement would be
// spending the latency budget blind.
func (s *Service) embedQuery(ctx context.Context, query string) (*pgvector.Vector, error) {
	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	embedResp, err := s.LLM.Embed(embedCtx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embedResp.Embeddings) == 0 {
		// An empty vector list is a failed embed, not an empty result set. It
		// has to surface as an error or the caller reports full-quality zero
		// hits for a query the vector leg never actually ran.
		return nil, errors.New("catalog: embed returned no vectors")
	}
	embedding := pgvector.NewVector(embedResp.Embeddings[0])
	return &embedding, nil
}

// hybridSearch runs the ADR-013 hybrid retrieval SQL: vector distance ranks,
// FTS widens candidates, anything past MaxCosineDistance is dropped rather than
// shown, and the DISC-003 filters narrow what survives.
//
// Scope is fixed to catalog workspaces inside the query itself (CORE-006) — an
// anonymous caller has no session to derive a scope from and must never supply
// one. The filters are the only caller-supplied predicates, and they can only
// ever narrow that scope.
func (s *Service) hybridSearch(ctx context.Context, queries *gen.Queries, query string, embedding *pgvector.Vector, limit int32, filters searchFilters) ([]searchResult, error) {
	rows, err := queries.PublicHybridSearchSkills(ctx, gen.PublicHybridSearchSkillsParams{
		Query:          query,
		QueryEmbedding: embedding,
		MaxDistance:    MaxCosineDistance,
		ResultLimit:    limit,
		HasScript:      filters.HasScript,
		SpecValidated:  filters.SpecValidated,
		AgentRuntime:   filters.AgentRuntime,
	})
	if err != nil {
		return nil, err
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
		// A row that arrived through the lexical leg with no embedding was never
		// measured against the query. Reporting 0 for it said "measured, and as
		// far away as possible", which is a different and false statement.
		if !row.Unranked {
			rank := row.Rank
			hit.Rank = &rank
		} else {
			hit.RankNote = rankNotePendingItem
		}
		resultFacets(&hit, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, nil
}

// ftsOnlySearch is the degradation path when the LLM service is unavailable.
// Same catalog-only scope as the hybrid path; the degraded path must not be
// the one that leaks (CORE-006).
//
// Every row comes back with a null rank. The page is ordered by ts_rank_cd,
// which the query no longer returns: it is an unbounded lexical score, and the
// field it used to travel in is documented as a cosine similarity in 0..1.
func (s *Service) ftsOnlySearch(ctx context.Context, queries *gen.Queries, query string, limit int32, filters searchFilters) ([]searchResult, error) {
	rows, err := queries.PublicSearchSkills(ctx, gen.PublicSearchSkillsParams{
		Query:         query,
		ResultLimit:   limit,
		HasScript:     filters.HasScript,
		SpecValidated: filters.SpecValidated,
		AgentRuntime:  filters.AgentRuntime,
	})
	if err != nil {
		return nil, err
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
		resultFacets(&hit, row.Tags, row.Scan, row.VerifiedAt,
			measuredCompat(row.AgentCapability, row.AgentRuntime, row.AgentRuntimeImage, row.AgentMeasuredAt))
		hits = append(hits, hit)
	}
	return hits, nil
}

// matchReasons calls the LLM service once for the top 10 hits — one batched
// request, never one per candidate, because the reason step sits in the
// user-visible latency budget (NFR-004) and N calls would multiply it by N.
// Timeout/failure is non-fatal and returns nothing; applyMatchReasons then
// covers every hit with a template reason (ADR-013 section 3).
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

	reasonCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	resp, err := s.LLM.MatchReasons(reasonCtx, query, candidates)
	if err != nil {
		slog.Warn("match-reasons call failed, using template fallback", "error", err)
		return nil
	}
	return resp.Reasons
}

// SearchWorkspace is the lexical-only, session-scoped read behind
// GET /skills/search. The workspace is the caller's, resolved from the session
// by the handler and never taken from the request (iron rule 3).
func (s *Service) SearchWorkspace(ctx context.Context, workspaceID pgtype.UUID, query string, limit int32) ([]gen.SearchSkillsRow, error) {
	return gen.New(s.Pool).SearchSkills(ctx, gen.SearchSkillsParams{
		WorkspaceID: workspaceID,
		Query:       query,
		Limit:       limit,
	})
}
