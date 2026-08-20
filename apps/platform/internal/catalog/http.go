// Package catalog owns skill discovery (ADR-002 Catalog & Discovery).
// Hybrid retrieval: pgvector similarity ranks, Postgres FTS only widens the
// candidate set (ADR-013 定案調整 3, measured in golden-query-set.md §3.7).
// The Python LLM service provides embeddings and match-reason generation;
// Go owns policy, auth, state, and retry (ADR-016 Iron Rule 6).
package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/httpx"
)

// Handler is the HTTP surface of discovery: request parsing, workspace scope
// resolution (iron rule 3) and status-code mapping. Everything below that lives
// on Service (service.go).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
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
// docs/plans/mvp/m1/golden-query-set.md §10.5 — the re-derivation against real
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
// that was ever validated, and says unverified.
//
// capability and runtime are the 0022 measurement for the row's newest version,
// COALESCEd to `unverified` in SQL for rows nothing has measured — that is
// DISC-002's 尚未試跑, and it is stated rather than omitted because a missing
// field reads as "fine". The runtime image travels with them: the verdict is
// about a (version, image) pair, and the same package answers differently on an
// image with a different set of interpreters.
func resultFacets(r *searchResult, tagsJSON, scanJSON []byte, verifiedAt pgtype.Timestamptz, compat compatibility) {
	r.Tier = tierLabel()
	r.Dependencies = dependencyTags(tagsJSON)
	r.Risk = riskHint(scanJSON)
	r.VerifiedAt = timeString(verifiedAt)
	r.Compat = compat
	r.Compat.SpecValidation = "unverified"
	if verifiedAt.Valid {
		r.Compat.SpecValidation = "passed"
	}
	r.Compat.Note = compatUnverifiedNote
	if r.Compat.RuntimeImage != "" {
		r.Compat.Note = compatMeasuredNote
	}
}

// measuredCompat builds the sandbox half of the compatibility block from the
// projected measurement columns. Empty RuntimeImage is how "no row" arrives (the
// SQL COALESCEs it), and it is what tells resultFacets which note to attach.
func measuredCompat(capability, runtime, image string, measuredAt pgtype.Timestamptz) compatibility {
	return compatibility{
		Capability:   capability,
		Runtime:      runtime,
		RuntimeImage: image,
		MeasuredAt:   timeString(measuredAt),
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

// --- DISC-003 structured filters --------------------------------------------

// searchFilters is the filter set as it reaches SQL. A nil pointer is "this
// dimension is not filtered", which is a third state and not the same as false:
// `script=no` asks for rows the scan says carry no script, whereas no filter at
// all also admits rows nothing is known about.
type searchFilters struct {
	HasScript     *bool
	SpecValidated *bool
	// AgentRuntime is matched against the measured runtime verdict exactly, not
	// as a boolean: `unverified` is a real value a caller can ask for, and
	// "not native" would otherwise silently mean "transpiled, failed, or never
	// measured", which are three different things to a reader choosing a skill.
	AgentRuntime *string
}

func (f searchFilters) active() bool {
	return f.HasScript != nil || f.SpecValidated != nil || f.AgentRuntime != nil
}

// agentRuntimeValues are the values ?agent= accepts — the runtime axis of
// DISC-002's Agent dimension, and only that axis.
//
// The capability axis is displayed but not filterable, for the reason `tier` is
// not: every measured skill in the catalogue came back `activated` (45/45 in the
// M2 baseline), so a filter on it separates nothing. It becomes a filter when a
// `not_activated` row exists, and not before — offering a control that cannot
// change the page is the same lie as a control that silently does nothing.
var agentRuntimeValues = map[string]bool{
	"native": true, "transpiled": true, "failed": true, "unverified": true,
}

// unavailableFilters are the 02:DISC-002 dimensions the platform still has no
// per-row data for. They are rejected rather than ignored: a shared URL carrying
// ?category=data would otherwise come back as a full unfiltered page that looks
// filtered, which is the one failure mode a filter must not have.
//
// The note is the honest reason, per dimension, and it is what the UI shows on
// the disabled control:
//
//   - category — the three PDM-001 categories exist only as a curation judgement
//     in tools/content/seed-skills.json. Nothing persists them: the import path
//     takes a package, not a category, and a user-imported skill would have none
//     at all. Storing them is CONTENT-003 curation work, not search work.
//   - tier — every indexed row is TierIndexed and nothing records a curation
//     review yet (see tierLabel), so the dimension has exactly one value and
//     filtering on it cannot separate anything.
//   - mcp — no MCP signal is captured anywhere in the scan or the manifest;
//     remote MCP is out of the MVP first release (AGENTS.md 範圍注意).
//
// `agent` left this map when 0022 gave it per-row data (02:DISC-002 篩選維度的
// 允收階段 lists it as M2, 依 Sandbox 實測).
var unavailableFilters = map[string]string{
	"category": "類別尚未存入平台:目前只存在於策展清單,匯入流程不收此欄位(CONTENT-003)。",
	"tier":     "來源層級目前全目錄同為「已索引」,沒有可區分的值(PDM-002 人工精選審查尚未開始)。",
	"mcp":      "是否需要 MCP 沒有任何來源資料:靜態掃描與 manifest 都沒有這項訊號,遠端 MCP 也不在 MVP 首發。",
}

// parseFilters reads the DISC-003 query parameters. An unrecognised value is an
// error rather than a silent nil, for the same reason unavailableFilters is:
// the caller has to be able to trust that a filtered page was filtered.
func parseFilters(r *http.Request) (searchFilters, error) {
	q := r.URL.Query()
	for name, note := range unavailableFilters {
		if q.Has(name) {
			return searchFilters{}, errors.New("filter not available: " + name + " — " + note)
		}
	}
	var out searchFilters
	var err error
	if out.HasScript, err = triState(q.Get("script"), "yes", "no"); err != nil {
		return searchFilters{}, errors.New(`script must be "yes" or "no"`)
	}
	if out.SpecValidated, err = triState(q.Get("validation"), "passed", "unverified"); err != nil {
		return searchFilters{}, errors.New(`validation must be "passed" or "unverified"`)
	}
	if v := q.Get("agent"); v != "" {
		if !agentRuntimeValues[v] {
			return searchFilters{}, errors.New(`agent must be "native", "transpiled", "failed" or "unverified"`)
		}
		out.AgentRuntime = &v
	}
	return out, nil
}

// triState maps an absent/yes/no query value onto nil/true/false.
func triState(v, yes, no string) (*bool, error) {
	switch v {
	case "":
		return nil, nil
	case yes:
		t := true
		return &t, nil
	case no:
		f := false
		return &f, nil
	}
	return nil, errors.New("unrecognised filter value")
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
	// FilteredOut says the query did match skills and the active DISC-003
	// filters removed all of them.
	//
	// Kept apart from NoResults because the two need opposite answers from the
	// user. NoResults means the catalogue has nothing close enough, and the
	// suggestion is to describe the task differently; FilteredOut means the
	// catalogue does have matches and the suggestion is to widen the filters.
	// Telling someone to rewrite their query when the real problem is a checkbox
	// they set is worse than saying nothing, so the two never share copy and
	// NoResults is false whenever this is true.
	FilteredOut bool `json:"filtered_out"`
}

// PublicSearch handles GET /api/skills/search?q=...&limit=N.
// This is the DISC-001 public search endpoint: works without login.
// Uses hybrid retrieval (ADR-013) when embeddings are available.
func (h *Handler) PublicSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "query parameter q must be at most 2000 characters")
		return
	}

	// DISC-003 filters are parsed before the query is even looked at: a request
	// asking for a filter this build cannot honour is rejected whatever the
	// query says, rather than being answered with an unfiltered page.
	filters, err := parseFilters(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

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

	out, err := h.Svc.Search(r.Context(), q, limit, filters)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "search failed")
		return
	}

	resp := searchResponse{
		Query:          q,
		Results:        out.Hits,
		Degraded:       out.DegradedReason != "",
		DegradedReason: out.DegradedReason,
		PartialIndex:   anyUnranked(out.Hits),
		FilteredOut:    out.FilteredOut,
	}
	// Everything fell outside MaxCosineDistance (or the lexical floor matched
	// nothing). Saying so explicitly is the point: a degraded empty answer still
	// carries its degraded flag, so the caller can tell "nothing is close
	// enough" from "we could not look properly".
	//
	// Not when the filters emptied the page: the catalogue did have answers, and
	// the refine-your-query suggestion would be advice about the wrong thing.
	if len(out.Hits) == 0 && !out.FilteredOut {
		resp.NoResults = true
		resp.QuerySuggestion = noResultsSuggestion
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
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

	rows, err := h.Svc.SearchWorkspace(r.Context(), ws.ID, q, limit)
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
