// Package catalog owns skill discovery (ADR-002 Catalog & Discovery).
// Hybrid retrieval: pgvector similarity ranks, Postgres FTS only widens the
// candidate set (ADR-013 定案調整 3, measured in golden-query-set.md §3.7).
// The Python LLM service provides embeddings and match-reason generation;
// Go owns policy, auth, state, and retry (ADR-016 Iron Rule 6).
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	// Store reads stored packages for the detail and file views (detail.go).
	// nil = those views report the package scan as unavailable rather than clean.
	Store ObjectStore
}

// Match reason provenance (DISC-002). ADR-013 requires model-generated content
// to be labelled as such, so the caller can tell an explanation the model wrote
// from one the platform assembled from the hit itself.
const (
	reasonSourceModel    = "model"
	reasonSourceTemplate = "template"
)

// MaxCosineDistance is the DISC-001/DISC-005 "no results" cut-off: a candidate
// further than this from the query is not returned at all, and a query where
// everything is further returns the no-results state instead of a page of
// nearest-but-irrelevant skills.
//
// 0.75 cosine distance is 0.25 cosine similarity, from
// plans/mvp/m1/golden-query-set.md §10.5 — the re-derivation against real
// index-time LLM enrichment, which is what this pipeline writes. Over that
// corpus the two distributions do not overlap: the best hit for an off-topic
// query peaked at 0.219 similarity, the worst genuine answer sat at 0.290, and
// this value is the midpoint of that clean gap. It refused 12/12 off-topic
// queries with 0/48 real answers lost.
//
// It replaces the 0.79 derived in §4.3 from bare frontmatter. Enrichment lifted
// both distributions but lifted genuine answers further (+0.059 against +0.031),
// so the safe gap widened from 0.043 to 0.071 rather than closing. Carrying 0.79
// over would have refused only 8/12 off-topic queries — under the 75% ADR-013
// requires — which is exactly the decay §4.4 predicted.
//
// This number expires. §4.4 names three changes that each invalidate it and
// require re-running tools/goldenset/evaluate.py before this constant is
// trusted again. Two are still live:
//
//  1. Corpus growth. The off-topic ceiling is a maximum over the corpus, so it
//     drifts up as the catalogue grows while the genuine-answer floor does not.
//     Re-measure every time the indexed document count grows by an order of
//     magnitude.
//  2. Index content. SETTLED for now: §10 measured the real enrichment output.
//     It reopens if the enrichment prompt or its model changes, because the
//     indexed text is then no longer what this value was derived against —
//     rerun `enrich_corpus.py` then `evaluate.py --index-mode enriched`.
//  3. Query rewriting. Still open. The measurement embedded the user's raw
//     sentence, which is the degraded path. A rewritten query has a different
//     distribution, so turning rewriting on needs either two cut-offs or one
//     union of both.
//
// It is also bound to text-embedding-3-small at 1536 dimensions; a different
// embedding model invalidates it outright (ADR-013).
const MaxCosineDistance = 0.75

// noResultsSuggestion is the DISC-001 prompt to refine. It asks for the three
// things the golden set found missing from the queries that failed: the format,
// the input in hand, and the wanted output.
const noResultsSuggestion = "No skill in the catalog is close enough to this task. " +
	"Try naming the file format you have, the input you are starting from, and the output you want."

// searchResult is the public JSON shape for each search hit (DISC-002: name,
// plain summary, source tier, agent compatibility, dependencies, risk hint and
// last verification time).
type searchResult struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Rank is cosine similarity in 0..1, or null when this page was not ranked
	// by similarity at all. RankNote then says what ordered it instead.
	//
	// The lexical score is never substituted: ts_rank_cd is unbounded (a local
	// answer measured 1.4) and is not the same quantity as a cosine distance, so
	// normalising it into 0..1 would manufacture a similarity that was never
	// computed. Withholding the number costs nothing the caller needs — the
	// array order still carries the ranking.
	Rank              *float64      `json:"rank"`
	RankNote          string        `json:"rank_note,omitempty"`
	Tier              labelled      `json:"tier"`
	Risk              searchRisk    `json:"risk"`
	Dependencies      []string      `json:"dependencies"`
	Compat            compatibility `json:"compatibility"`
	VerifiedAt        string        `json:"verified_at,omitempty"`
	MatchReason       string        `json:"match_reason,omitempty"`
	MatchReasonSource string        `json:"match_reason_source,omitempty"`
	// unranked: this document has no embedding yet (enrichment_status
	// 'pending'), so it reached the page through the lexical leg and the
	// distance cut-off never judged it. Internal — it is reported once at the
	// envelope level as PartialIndex, and per row as a null Rank.
	unranked bool
}

// searchRisk is the DISC-002 risk hint: what the import scan recorded, folded
// small enough for a list row. The detail view re-scans the package and reports
// findings verbatim; this reads the projection, so it can be shown for a whole
// page without one object-store fetch per hit.
type searchRisk struct {
	// ScanStatus is scanned | unavailable. Unavailable means the projection
	// carries no scan for this row, which is reported as unknown and never as
	// clean (DISC-004 不得自行推定為通過).
	ScanStatus string `json:"scan_status"`
	// Level is none | disclosed | warning. There is no "safe" level (NFR-001).
	Level    string `json:"level"`
	Warnings int    `json:"warnings"`

	HasScripts            bool `json:"has_scripts"`
	HasEmbeddedScript     bool `json:"has_embedded_script"`
	HasExternalURLs       bool `json:"has_external_urls"`
	HasPossibleSecrets    bool `json:"has_possible_secrets"`
	HasBinaries           bool `json:"has_binaries"`
	HasDependencyManifest bool `json:"has_dependency_manifest"`

	Note string `json:"note"`
}

const (
	riskLevelNone      = "none"
	riskLevelDisclosed = "disclosed"
	riskLevelWarning   = "warning"

	searchRiskNote      = "來自匯入時的靜態掃描,不執行套件內任何程式碼;開啟 Skill 可看逐項結果。"
	searchRiskUnknown   = "此結果尚無掃描紀錄,狀態未知——不代表已通過檢查。"
	rankNoteDegraded    = "此次搜尋未使用語意向量,排序來自關鍵字比對分數,與相似度不同量綱,因此不提供分數。"
	rankNotePendingItem = "此 Skill 尚未建立語意索引,是以關鍵字比對進入結果,未與查詢計算過相似度。"
)

// resultFacets fills the DISC-002 columns that are the same for every row of
// every page, and the ones read straight out of the projection.
//
// Tier is TierIndexed unconditionally, for the reason tierLabel() gives: nothing
// records a curation review yet, and index membership is not one.
//
// spec_validation is `passed` whenever the row has a saved version. That is not
// an assumption — skillpkg.Validate blocks the import on any error-level
// finding and a blocked package is never stored, so appearing in the index *is*
// the evidence that static validation passed. A row with no version has nothing
// that was ever validated, and says unverified. capability and runtime stay
// unverified until sandbox runs exist (M2); that is DISC-002's 尚未試跑, and it
// is stated rather than omitted because a missing field reads as "fine".
func resultFacets(r *searchResult, tagsJSON, scanJSON []byte, verifiedAt pgtype.Timestamptz) {
	r.Tier = tierLabel()
	r.Dependencies = dependencyTags(tagsJSON)
	r.Risk = riskHint(scanJSON)
	r.VerifiedAt = timeString(verifiedAt)
	r.Compat = compatibility{
		SpecValidation: "unverified",
		Capability:     "unverified",
		Runtime:        "unverified",
		Note:           compatNote,
	}
	if verifiedAt.Valid {
		r.Compat.SpecValidation = "passed"
	}
}

// dependencyTags reads the dependency bucket out of the stored enrichment tags.
// Absent, unparseable or pending enrichment all mean the same thing here: no
// dependency was extracted, which is not a claim that there are none.
func dependencyTags(tagsJSON []byte) []string {
	var t llmclient.SkillTags
	if len(tagsJSON) == 0 || json.Unmarshal(tagsJSON, &t) != nil || t.Dependencies == nil {
		return []string{}
	}
	return t.Dependencies
}

// riskHint turns the projected scan facts into the row's risk block.
func riskHint(scanJSON []byte) searchRisk {
	var f struct {
		Warnings int      `json:"warnings"`
		Codes    []string `json:"codes"`
	}
	if len(scanJSON) == 0 || json.Unmarshal(scanJSON, &f) != nil {
		return searchRisk{ScanStatus: "unavailable", Level: riskLevelNone, Note: searchRiskUnknown}
	}
	out := searchRisk{ScanStatus: "scanned", Warnings: f.Warnings, Note: searchRiskNote}
	for _, code := range f.Codes {
		switch code {
		case "script-file":
			out.HasScripts = true
		case "embedded-script":
			out.HasEmbeddedScript = true
		case "external-url":
			out.HasExternalURLs = true
		case "possible-secret":
			out.HasPossibleSecrets = true
		case "binary-file":
			out.HasBinaries = true
		case "dependency-file":
			out.HasDependencyManifest = true
		}
	}
	switch {
	case out.Warnings > 0:
		out.Level = riskLevelWarning
	case out.HasScripts || out.HasEmbeddedScript || out.HasExternalURLs ||
		out.HasPossibleSecrets || out.HasBinaries || out.HasDependencyManifest:
		out.Level = riskLevelDisclosed
	default:
		out.Level = riskLevelNone
	}
	return out
}

// searchResponse is the top-level JSON envelope.
type searchResponse struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`
	// Degraded says the vector leg did not run, so this answer came from
	// lexical matching alone (ADR-013 定案調整 1/2: the vector leg is what
	// carries cross-language recall, FTS-only is the availability floor).
	// Callers should treat a degraded answer as lower recall, not as "no such
	// skill exists".
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degraded_reason,omitempty"`
	// PartialIndex says at least one result on this page has no embedding yet
	// and therefore could not be ranked or judged by the distance cut-off — it
	// arrived through the lexical leg and sits at the bottom.
	//
	// Deliberately not folded into Degraded: enrichment failing for a package is
	// a normal, permanent-until-backfilled state of the catalogue (import never
	// fails on a model outage), whereas Degraded is a transient outage of the
	// query-side embedding call. One boolean covering both would tell the caller
	// neither (import-report.md §6.1 bug 2).
	PartialIndex bool `json:"partial_index"`
	// NoResults distinguishes "we searched and nothing was close enough" from
	// an empty list the caller has to interpret (DISC-001 清楚的無結果狀態).
	// Also true for a query that was never searched at all — blank or
	// incomprehensible — because the answer the user needs is the same one.
	NoResults bool `json:"no_results"`
	// QuerySuggestion is the copy to show alongside NoResults (DISC-001
	// 提示使用者補充任務、輸入或預期輸出). Empty when there are results.
	QuerySuggestion string `json:"query_suggestion,omitempty"`
}

// PublicSearch handles GET /api/skills/search?q=...&limit=N.
// This is the DISC-001 public search endpoint: works without login.
// Uses hybrid retrieval (ADR-013) when embeddings are available.
func (h *Handler) PublicSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	// DISC-001: blank/incomprehensible queries don't create search.
	if q == "" || !isComprehensible(q) {
		httpx.WriteJSON(w, http.StatusOK, searchResponse{
			Query:           q,
			Results:         []searchResult{},
			NoResults:       true,
			QuerySuggestion: noResultsSuggestion,
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

	// Hybrid retrieval is the intended path. Degrading to FTS-only is an
	// availability floor, not an equivalent alternative: ADR-013 定案調整 1
	// measured the vector leg carrying cross-language recall that the lexical
	// leg misses entirely, so a degraded answer is flagged rather than passed
	// off as a normal one. The reverse degradation (vector-only when FTS is
	// down) is not a case that exists: both legs live in the same database.
	var (
		hits           []searchResult
		degradedReason string
	)

	if h.LLMClient == nil {
		degradedReason = "embedding service not configured; lexical search only"
	} else if hybridHits, err := h.hybridSearch(ctx, queries, q, limit); err != nil {
		slog.Warn("hybrid search failed, falling back to FTS", "error", err)
		degradedReason = "embedding unavailable; lexical search only"
	} else {
		hits = hybridHits
	}

	if degradedReason != "" {
		hits, _ = h.ftsOnlySearch(ctx, queries, q, limit)
	}

	if hits == nil {
		hits = []searchResult{}
	}

	// DISC-002: one batched call for the whole result page, never one per hit.
	var reasons []llmclient.MatchReason
	if len(hits) > 0 && h.LLMClient != nil {
		reasons = h.matchReasons(ctx, q, hits)
	}
	applyMatchReasons(hits, q, reasons)

	resp := searchResponse{
		Query:          q,
		Results:        hits,
		Degraded:       degradedReason != "",
		DegradedReason: degradedReason,
		PartialIndex:   anyUnranked(hits),
	}
	// Everything fell outside MaxCosineDistance (or the lexical floor matched
	// nothing). Saying so explicitly is the point: a degraded empty answer still
	// carries its degraded flag, so the caller can tell "nothing is close
	// enough" from "we could not look properly".
	if len(hits) == 0 {
		resp.NoResults = true
		resp.QuerySuggestion = noResultsSuggestion
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// hybridSearch runs the ADR-013 hybrid retrieval pipeline:
//  1. Get query embedding from the LLM service
//  2. Run the hybrid SQL: vector distance ranks, FTS widens candidates, and
//     anything past MaxCosineDistance is dropped rather than shown
//
// ponytail: the raw query is embedded as typed. ADR-013 定案調整 2 puts LLM
// query rewriting ahead of the embedding call as a Top-1 precision gain (80% ->
// 100% in the spike), not as a recall requirement — its whole degradation path
// is "embed the original sentence", which is what this does unconditionally.
// Add the rewrite step once the end-to-end p95 budget (NFR-004) has been
// measured with a real gateway; adding it before that measurement would be
// spending the latency budget blind.
func (h *Handler) hybridSearch(ctx context.Context, queries *gen.Queries, query string, limit int32) ([]searchResult, error) {
	// Step 1: get query embedding.
	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	embedResp, err := h.LLMClient.Embed(embedCtx, []string{query})
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

	// Step 2: hybrid search with RRF. Scope is fixed to catalog workspaces
	// inside the query itself (CORE-006) — an anonymous caller has no session
	// to derive a scope from and must never supply one.
	rows, err := queries.PublicHybridSearchSkills(ctx, gen.PublicHybridSearchSkillsParams{
		Query:          query,
		QueryEmbedding: &embedding,
		MaxDistance:    MaxCosineDistance,
		ResultLimit:    limit,
	})
	if err != nil {
		return nil, err
	}

	hits := make([]searchResult, 0, len(rows))
	for _, row := range rows {
		hit := searchResult{
			SkillID:  uuidString(row.SkillID),
			Name:     row.Name,
			Summary:  row.Summary,
			unranked: row.Unranked,
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
		resultFacets(&hit, row.Tags, row.Scan, row.VerifiedAt)
		hits = append(hits, hit)
	}
	return hits, nil
}

// anyUnranked reports whether the page carries a not-yet-enriched document.
// Only the hybrid path can tell: on the FTS-only path nothing was ranked at all
// and Degraded already says so.
func anyUnranked(hits []searchResult) bool {
	for _, h := range hits {
		if h.unranked {
			return true
		}
	}
	return false
}

// ftsOnlySearch is the degradation path when the LLM service is unavailable.
// Same catalog-only scope as the hybrid path; the degraded path must not be
// the one that leaks (CORE-006).
//
// Every row comes back with a null rank. The page is ordered by ts_rank_cd,
// which the query no longer returns: it is an unbounded lexical score, and the
// field it used to travel in is documented as a cosine similarity in 0..1.
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
		hit := searchResult{
			SkillID:  uuidString(row.SkillID),
			Name:     row.Name,
			Summary:  row.Summary,
			RankNote: rankNoteDegraded,
		}
		resultFacets(&hit, row.Tags, row.Scan, row.VerifiedAt)
		hits = append(hits, hit)
	}
	return hits, nil
}

// matchReasons calls the LLM service once for the top 10 hits — one batched
// request, never one per candidate, because the reason step sits in the
// user-visible latency budget (NFR-004) and N calls would multiply it by N.
// Timeout/failure is non-fatal and returns nothing; applyMatchReasons then
// covers every hit with a template reason (ADR-013 section 3).
func (h *Handler) matchReasons(ctx context.Context, query string, hits []searchResult) []llmclient.MatchReason {
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

	resp, err := h.LLMClient.MatchReasons(reasonCtx, query, candidates)
	if err != nil {
		slog.Warn("match-reasons call failed, using template fallback", "error", err)
		return nil
	}
	return resp.Reasons
}

// applyMatchReasons gives every hit a reason and the provenance label that
// actually matches where that sentence came from (DISC-002, ADR-013: model
// output must be marked as model output).
//
// The labelling is per candidate, never per batch. A batch that fails, times
// out, or covers only some of the page leaves the rest on templateMatchReason,
// and those must not inherit the `model` label from their neighbours — that is
// exactly how the LLM service's own filler sentences ended up presented as
// model-written explanations (import-report.md §6.1 bug 3).
func applyMatchReasons(hits []searchResult, query string, reasons []llmclient.MatchReason) {
	fromModel := make(map[string]string, len(reasons))
	for _, r := range reasons {
		if r.Reason != "" {
			fromModel[r.SkillID] = r.Reason
		}
	}
	for i := range hits {
		if reason, ok := fromModel[hits[i].SkillID]; ok {
			hits[i].MatchReason = reason
			hits[i].MatchReasonSource = reasonSourceModel
			continue
		}
		hits[i].MatchReason = templateMatchReason(hits[i].Name, hits[i].Summary, query)
		hits[i].MatchReasonSource = reasonSourceTemplate
	}
}

// templateMatchReason is the zero-latency fallback reason (ADR-013 section 3):
// it states the actual lexical overlap between the query and the skill's own
// text, so the explanation is derived from the hit rather than invented.
//
// When there is no overlap it says so. ADR-013 定案調整 4 found that
// no-lexical-overlap hits are the *main* case in this product — the vector leg
// recalls Chinese task descriptions against English SKILL.md files — and that a
// template has nothing truthful to say about them. Stitching the summary onto
// the query anyway (what this used to do) produced a sentence that looked like
// a reason and contained none, which is worse than admitting the ranking came
// from semantic similarity.
func templateMatchReason(name, summary, query string) string {
	if terms := overlapTerms(query, name+" "+summary); len(terms) > 0 {
		return "Matches your query on: " + strings.Join(terms, ", ") + "."
	}
	return "No shared keywords — ranked by semantic similarity to your task description."
}

// overlapTerms returns the tokens the query and the document share, in query
// order, capped so the reason stays one readable line.
func overlapTerms(query, doc string) []string {
	inDoc := make(map[string]bool)
	for _, t := range tokenize(doc) {
		inDoc[t] = true
	}
	var out []string
	seen := make(map[string]bool)
	for _, t := range tokenize(query) {
		if !inDoc[t] || seen[t] || stopwords[t] {
			continue
		}
		seen[t] = true
		// Rejoin CJK bigrams the tokenizer split out of one phrase, so a match
		// on 資料分析 reads as that and not as "資料, 料分, 分析".
		if n := len(out); n > 0 && overlapsByOneRune(out[n-1], t) {
			out[n-1] += lastRune(t)
			continue
		}
		out = append(out, t)
		if len(out) == 5 {
			break
		}
	}
	return out
}

// stopwords are the words whose presence in both texts says nothing. Kept to
// the handful that actually show up in task descriptions; tokenize already
// drops every latin token shorter than three runes.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "from": true, "into": true, "you": true, "your": true,
	"can": true, "are": true, "use": true, "using": true, "how": true,
}

// tokenize splits text into comparable tokens. Latin runs become whole words;
// CJK runs become character bigrams, because Chinese queries carry no spaces
// and single characters collide far too often to evidence anything.
func tokenize(s string) []string {
	var out []string
	var latin, cjk []rune
	flushLatin := func() {
		if len(latin) >= 3 {
			out = append(out, string(latin))
		}
		latin = latin[:0]
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 1:
			out = append(out, string(cjk))
		default:
			for i := 0; i+1 < len(cjk); i++ {
				out = append(out, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case isCJK(r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin = append(latin, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return out
}

// isCJK covers the ranges a zh-Hant task description actually lands in: the
// unified ideographs plus the extension A block.
func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF)
}

func overlapsByOneRune(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	if len(ar) < 2 || len(br) != 2 || !isCJK(br[0]) {
		return false
	}
	return ar[len(ar)-1] == br[0]
}

func lastRune(s string) string {
	r := []rune(s)
	return string(r[len(r)-1])
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

	// No score: this endpoint has only the lexical leg, and ts_rank_cd is not a
	// similarity (see searchResult.Rank). Best first is already the array order.
	type searchHit struct {
		SkillID string `json:"skill_id"`
		Name    string `json:"name"`
		Summary string `json:"summary"`
	}
	hits := make([]searchHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, searchHit{
			SkillID: uuidString(row.SkillID),
			Name:    row.Name,
			Summary: row.Summary,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": hits})
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
