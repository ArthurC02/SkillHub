// Package catalog owns skill discovery (ADR-002 Catalog & Discovery).
// Hybrid retrieval: FTS + pgvector vector similarity fused with RRF (ADR-013).
// The Python LLM service provides embeddings and match-reason generation;
// Go owns policy, auth, state, and retry (ADR-016 Iron Rule 6).
package catalog

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
)

type Handler struct {
	Pool      *pgxpool.Pool
	Identity  *identity.Service
	LLMClient *llmclient.Client // nil = embedding unavailable, FTS-only fallback
}

// searchResult is the public JSON shape for each search hit (DISC-002).
type searchResult struct {
	SkillID     string  `json:"skill_id"`
	Name        string  `json:"name"`
	Summary     string  `json:"summary"`
	Rank        float64 `json:"rank"`
	MatchReason string  `json:"match_reason,omitempty"`
}

// searchResponse is the top-level JSON envelope.
type searchResponse struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`
}

// PublicSearch handles GET /api/skills/search?q=...&limit=N.
// This is the DISC-001 public search endpoint: works without login.
// Uses hybrid retrieval (ADR-013) when embeddings are available.
func (h *Handler) PublicSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	// DISC-001: blank/incomprehensible queries don't create search.
	if q == "" || !isComprehensible(q) {
		httpx.WriteJSON(w, http.StatusOK, searchResponse{
			Query:   q,
			Results: []searchResult{},
		})
		return
	}

	limit := int32(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}

	ctx := r.Context()
	queries := gen.New(h.Pool)

	// Try hybrid search with embeddings; fall back to FTS-only if LLM service
	// is unavailable or embedding call fails (ADR-013 degradation path).
	var hits []searchResult

	if h.LLMClient != nil {
		hybridHits, err := h.hybridSearch(ctx, queries, q, limit)
		if err != nil {
			slog.Warn("hybrid search failed, falling back to FTS", "error", err)
			hits, _ = h.ftsOnlySearch(ctx, queries, q, limit)
		} else {
			hits = hybridHits
		}
	} else {
		hits, _ = h.ftsOnlySearch(ctx, queries, q, limit)
	}

	if hits == nil {
		hits = []searchResult{}
	}

	// DISC-002: generate match reasons for top results.
	if len(hits) > 0 && h.LLMClient != nil {
		h.enrichMatchReasons(ctx, q, hits)
	}

	// DISC-002: fill template fallback for any hit still without a reason.
	for i := range hits {
		if hits[i].MatchReason == "" {
			hits[i].MatchReason = templateMatchReason(hits[i].Name, hits[i].Summary, q)
		}
	}

	httpx.WriteJSON(w, http.StatusOK, searchResponse{
		Query:   q,
		Results: hits,
	})
}

// hybridSearch runs the ADR-013 hybrid retrieval pipeline:
// 1. Get query embedding from the LLM service
// 2. Run hybrid SQL (FTS + vector + RRF)
func (h *Handler) hybridSearch(ctx context.Context, queries *gen.Queries, query string, limit int32) ([]searchResult, error) {
	// Step 1: get query embedding.
	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	embedResp, err := h.LLMClient.Embed(embedCtx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embedResp.Embeddings) == 0 {
		return nil, err
	}

	embedding := pgvector.NewVector(embedResp.Embeddings[0])

	// Step 2: hybrid search with RRF. Scope is fixed to catalog workspaces
	// inside the query itself (CORE-006) — an anonymous caller has no session
	// to derive a scope from and must never supply one.
	rows, err := queries.PublicHybridSearchSkills(ctx, gen.PublicHybridSearchSkillsParams{
		Query:          query,
		QueryEmbedding: &embedding,
		ResultLimit:    limit,
	})
	if err != nil {
		return nil, err
	}

	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, searchResult{
			SkillID: uuidString(row.SkillID),
			Name:    row.Name,
			Summary: row.Summary,
			Rank:    row.Rank,
		})
	}
	return hits, nil
}

// ftsOnlySearch is the degradation path when the LLM service is unavailable.
// Same catalog-only scope as the hybrid path; the degraded path must not be
// the one that leaks (CORE-006).
func (h *Handler) ftsOnlySearch(ctx context.Context, queries *gen.Queries, query string, limit int32) ([]searchResult, error) {
	rows, err := queries.PublicSearchSkills(ctx, gen.PublicSearchSkillsParams{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, searchResult{
			SkillID: uuidString(row.SkillID),
			Name:    row.Name,
			Summary: row.Summary,
			Rank:    row.Rank,
		})
	}
	return hits, nil
}

// enrichMatchReasons calls the LLM service to generate match reasons for up
// to the top 10 hits. Timeout/failure is non-fatal: template reasons are used
// as fallback (ADR-013 section 3).
func (h *Handler) enrichMatchReasons(ctx context.Context, query string, hits []searchResult) {
	n := len(hits)
	if n > 10 {
		n = 10
	}

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

	resp, err := h.LLMClient.MatchReasons(reasonCtx, query, candidates)
	if err != nil {
		slog.Warn("match-reasons call failed, using template fallback", "error", err)
		return
	}

	// Build a lookup map from skill_id to reason.
	reasonMap := make(map[string]string, len(resp.Reasons))
	for _, r := range resp.Reasons {
		reasonMap[r.SkillID] = r.Reason
	}

	for i := 0; i < n; i++ {
		if reason, ok := reasonMap[hits[i].SkillID]; ok && reason != "" {
			hits[i].MatchReason = reason
		}
	}
}

// templateMatchReason generates a simple template-based match reason when
// LLM polishing is unavailable or timed out (ADR-013 section 3 fallback).
func templateMatchReason(name, summary, query string) string {
	if summary != "" {
		// Truncate summary for the reason if too long.
		s := summary
		if len(s) > 120 {
			s = s[:120] + "..."
		}
		return name + " — " + s
	}
	return name + " may be relevant to your task."
}

// isComprehensible does a minimal check that the query contains at least one
// meaningful word or CJK character (DISC-001: incomprehensible queries don't
// create search).
func isComprehensible(q string) bool {
	if len(q) < 2 {
		return false
	}
	wordChars := 0
	for _, r := range q {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r >= 0x4E00 {
			wordChars++
		}
	}
	return wordChars >= 2 || utf8.RuneCountInString(q) >= 2
}

// Search handles GET /skills/search?q=...&limit=N. Session-scoped to the
// caller's workspace; the public curated scope arrives with CONTENT work.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "query parameter q is required")
		return
	}
	limit := int32(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}

	rows, err := gen.New(h.Pool).SearchSkills(r.Context(), gen.SearchSkillsParams{
		WorkspaceID: ws.ID,
		Query:       q,
		Limit:       limit,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "search failed")
		return
	}

	type searchHit struct {
		SkillID string  `json:"skill_id"`
		Name    string  `json:"name"`
		Summary string  `json:"summary"`
		Rank    float64 `json:"rank"`
	}
	hits := make([]searchHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, searchHit{
			SkillID: uuidString(row.SkillID),
			Name:    row.Name,
			Summary: row.Summary,
			Rank:    row.Rank,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": hits})
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
