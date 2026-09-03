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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
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
// It is also bound to text-embedding-3-small at 1536 dimensions // one-number: embeddingDimensions
// — a different embedding model invalidates it outright (ADR-013).
const MaxCosineDistance = 0.75

// noResultsSuggestion is the DISC-001 prompt to refine. It asks for the three
// things the golden set found missing from the queries that failed: the task
// itself, the input in hand, and the wanted output. DISC-001 條 3 names those
// three; the earlier wording opened on the file format and left the task
// implicit, which is two and a half of them.
//
// It no longer restates "沒有夠接近的 Skill" either. That sentence is rendered
// by the page above this one (Home.tsx) and is GEN-004's anchor for the
// generation entry point, so repeating it here would put it on screen twice and
// give the anchor two candidates.
//
// Written in the interface language because the front end renders this string
// verbatim and keeps no translation table of its own (the reasoning is recorded
// at WorkspaceRuns.tsx:65-83): whatever the server sends is what the user reads.
const noResultsSuggestion = "試著補充三件事:你想完成的任務、你手上已經有的輸入," +
	"以及你預期得到的輸出。"

// searchResult is the public JSON shape for each search hit (DISC-002: name,
// plain summary, source tier, agent compatibility, dependencies, risk hint and
// last verification time).
type searchResult struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// SummarySource is `model` or `package`, and it exists because this row was
	// already labelling the model's `match_reason` three fields down while
	// printing the model's rewrite of the summary unmarked (ADR-013). The
	// summary is the sentence a reader decides on; the match reason is the
	// footnote. Marking the footnote and not the sentence is the wrong way
	// round.
	SummarySource string `json:"summary_source"`
	// Rank is cosine similarity in 0..1, or null when this page was not ranked
	// by similarity at all. RankNote then says what ordered it instead.
	//
	// The lexical score is never substituted: ts_rank_cd is unbounded (a local
	// answer measured 1.4) and is not the same quantity as a cosine distance, so
	// normalising it into 0..1 would manufacture a similarity that was never
	// computed. Withholding the number costs nothing the caller needs — the
	// array order still carries the ranking.
	Rank     *float64 `json:"rank"`
	RankNote string   `json:"rank_note,omitempty"`
	Tier     labelled `json:"tier"`
	// Category is the PDM-001 shelf (0053). Required and always present, because
	// the absence has a name of its own — `unassigned` / 尚未定值 — and an omitted
	// field would put that decision back on whatever renders the row, which is
	// how 設計 §2.9's failure mode gets in at the contract layer. It sits next to
	// Tier and answers the opposite question: what this is for, not how much
	// review it has had.
	Category          labelled      `json:"category"`
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
	// Level is unknown | none | disclosed | warning. There is no "safe" level
	// (NFR-001), and `unknown` exists because `none` was being used for two
	// different facts: "the scan found nothing to disclose" and "there is no scan".
	// `none` is the lowest rung of a three-value ladder, so a client that reads
	// only this field — or a reader who only sees the colour — read 「沒有掃描紀錄」
	// as 「掃過了，沒事」, which is exactly what DISC-004 forbids. ScanStatus and
	// Note were telling the truth the whole time; this field was contradicting them.
	Level    string `json:"level"`
	Warnings int    `json:"warnings"`

	// Disclosures is the same catalogue the detail view serves (disclosure.go).
	// Same list, same order, same words — which is the entire point: these two
	// used to be separate boolean sets of different size (04 丙-29 ④).
	Disclosures []disclosure `json:"disclosures"`

	Note string `json:"note"`
}

const (
	// riskLevelUnknown is not a rung on the ladder: it means the ladder does
	// not apply because nothing was measured. Never rendered as a low risk.
	riskLevelUnknown   = "unknown"
	riskLevelNone      = "none"
	riskLevelDisclosed = "disclosed"
	riskLevelWarning   = "warning"

	searchRiskNote      = "來自匯入時的靜態掃描,不執行套件內任何程式碼;開啟 Skill 可看逐項結果。"
	searchRiskUnknown   = "此結果尚無掃描紀錄,狀態未知——不代表已通過檢查。"
	rankNoteDegraded    = "此次搜尋未使用語意向量,排序來自關鍵字比對分數,與相似度不同量綱,因此不提供分數。"
	rankNotePendingItem = "此 Skill 尚未建立語意索引,是以關鍵字比對進入結果,未與查詢計算過相似度。"
	rankNoteCatalog     = "這是目錄本身,不是某一句話的搜尋結果,所以沒有相似度可以顯示;排序是精選在前、其餘依版本建立時間由新到舊。"
)

// resultFacets fills the DISC-002 columns that are the same for every row of
// every page, and the ones read straight out of the projection.
//
// Tier arrives already resolved from SQL: the query compares the skill's
// curated_version_id against its newest version, so a curated skill whose
// content moved on comes back as 已索引 without anything here having to know
// the rule. See 0042 and the `cur` lateral in search.sql.
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
// category arrives as the raw nullable column, not resolved: NULL is a state
// with its own word (尚未定值) and the SQL deliberately does not COALESCE it into
// a shelf nobody chose.
func resultFacets(r *searchResult, tier string, category *string, tagsJSON, scanJSON []byte, verifiedAt pgtype.Timestamptz, compat compatibility) {
	r.Tier = tierLabel(Tier(tier))
	r.Category = categoryLabel(category)
	r.Dependencies = dependencyTags(tagsJSON)
	r.Risk = riskHint(scanJSON)
	r.VerifiedAt = timeString(verifiedAt)
	r.Compat = compat
	r.Compat.SpecValidation = axis(specWords, "unverified")
	if verifiedAt.Valid {
		r.Compat.SpecValidation = axis(specWords, "passed")
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
		Capability:   axis(capabilityWords, capability),
		Runtime:      axis(runtimeWords, runtime),
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
		return searchRisk{
			ScanStatus:  "unavailable",
			Level:       riskLevelUnknown,
			Disclosures: []disclosure{},
			Note:        searchRiskUnknown,
		}
	}
	out := searchRisk{ScanStatus: "scanned", Warnings: f.Warnings, Note: searchRiskNote}
	codes := map[string]bool{}
	for _, code := range f.Codes {
		codes[code] = true
	}
	out.Disclosures = disclosuresFor(codes)
	switch {
	case out.Warnings > 0:
		out.Level = riskLevelWarning
	// Any disclosure at all, rather than six named booleans OR-ed together: a
	// seventh finding code added to the catalogue is then disclosed here without
	// anyone remembering to extend this condition.
	case len(out.Disclosures) > 0:
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
	// CurationTier is matched against the resolved verdict, so `indexed` asks
	// for everything that is not currently curated — including a skill whose
	// curated version has been superseded. That is the honest reading: the
	// question a filter answers is "what am I looking at now".
	CurationTier *string
	// Category is matched against the stored PDM-001 shelf (0053). There is no
	// `unassigned` value to ask for: NULL is the platform not having decided,
	// and a filter for it would offer 「show me the rows nobody classified」 as if
	// that were a shelf. Absent means every row, classified or not.
	Category *string
}

func (f searchFilters) active() bool {
	return f.HasScript != nil || f.SpecValidated != nil || f.AgentRuntime != nil ||
		f.CurationTier != nil || f.Category != nil
}

// curationTierValues are the values ?tier= accepts. TierExternal is not one of
// them: an external result was never imported, so it has no row to filter.
var curationTierValues = map[string]bool{
	string(TierCurated): true, string(TierIndexed): true,
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

// categoryValues are the values ?category= accepts — the three PDM-001 shelves
// and only those. `unassigned` is not among them for the reason
// searchFilters.Category gives: it is the platform's own undecided state, not a
// shelf, and 0053 stores it as NULL.
var categoryValues = map[string]bool{
	string(CategoryDocuments): true, string(CategoryWriting): true, string(CategoryData): true,
}

// unavailableFilters are the 02:DISC-002 dimensions the platform still has no
// per-row data for. They are rejected rather than ignored: a shared URL carrying
// ?category=data would otherwise come back as a full unfiltered page that looks
// filtered, which is the one failure mode a filter must not have.
//
// The note is the honest reason, per dimension, and it is what the UI shows on
// the disabled control:
//
//   - mcp — no MCP signal is captured anywhere in the scan or the manifest;
//     remote MCP is out of the MVP first release (AGENTS.md 範圍注意).
//
// Three dimensions have left this map, each when a migration gave it per-row
// data, and the history is worth keeping because the notes were all true when
// they were written and every one of them outlived its truth by some days:
//
//   - `agent` left when 0022 measured it (02:DISC-002 篩選維度的允收階段 lists it
//     as M2, 依 Sandbox 實測).
//   - `tier` left when 0042 gave the catalogue a second value — the note here
//     stayed true for twelve days after the review that would have made it false
//     had already been recorded somewhere else.
//   - `category` left on 2026-09-03, when 0053 persisted the three PDM-001
//     shelves onto `skills.category`. Its note said 「類別尚未存入平台:目前只存在於
//     策展清單,匯入流程不收此欄位(CONTENT-003)」, and every clause of that was a
//     fact until the column existed. What 0053 did NOT settle is 05 R-19 — how a
//     user-imported Skill gets a category — so those rows are NULL and read as
//     尚未定值. That is a value the reader is told about, not a refused filter.
var unavailableFilters = map[string]string{
	"mcp": "是否需要 MCP 沒有任何來源資料:靜態掃描與 manifest 都沒有這項訊號,遠端 MCP 也不在 MVP 首發。",
}

// parseLimit reads the `limit` both search endpoints declare identically in
// public.yaml: `{ type: integer, minimum: 1, maximum: 100, default: 20 }`.
// Absent is the default; present is the schema or it is a 400.
//
// It used to be the one schema violation these handlers swallowed: `limit=0`,
// `limit=500` and `limit=abc` all fell back to 20, in the same function that
// answers 400 to an over-long `q` and to an unusable filter. What that cost the
// caller is not pedantry — someone asking for 500 got 20 rows and
// `truncated=true`, and read it as "the catalogue holds a bit over 20". ADR-042
// 決策 3 requires a truncation to declare itself, and the thing declaring itself
// there was a ceiling the caller never asked for (M1/M5 audit, 2026-08-25).
//
// Both bounds are inclusive, which is what JSON Schema's minimum/maximum mean:
// 1 and 100 are accepted, 0 and 101 are not. `limit=` (present, empty) is
// refused too — `allowEmptyValue` is not set on the parameter, so an empty
// string is not one of the integers the schema describes.
func parseLimit(r *http.Request) (int32, error) {
	q := r.URL.Query()
	if !q.Has("limit") {
		return 20, nil
	}
	n, err := strconv.Atoi(q.Get("limit"))
	if err != nil || n < 1 || n > 100 {
		return 0, errors.New("query parameter limit must be a whole number between 1 and 100")
	}
	return int32(n), nil
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
	for _, name := range []string{"script", "validation", "agent", "tier", "category"} {
		if q.Has(name) && q.Get(name) == "" {
			return searchFilters{}, errors.New(name + " must not be empty")
		}
	}
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
	if v := q.Get("tier"); v != "" {
		if !curationTierValues[v] {
			return searchFilters{}, errors.New(`tier must be "curated" or "indexed"`)
		}
		out.CurationTier = &v
	}
	if v := q.Get("category"); v != "" {
		if !categoryValues[v] {
			return searchFilters{}, errors.New(`category must be "documents", "writing" or "data"`)
		}
		out.Category = &v
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
	// Limit and Truncated are the cap saying so (ADR-042 決策 3). The cap has
	// always been here; what was missing is that hit 21 did not exist as far as
	// the caller could tell, and a page that is quietly short reads as the whole
	// answer. Distinct from PartialIndex and Degraded on purpose: those are
	// statements about how well we could look, this one is about how much of what
	// we found is on the page.
	Limit     int32 `json:"limit"`
	Truncated bool  `json:"truncated"`
	// Total is 設計系統 §4.3's other half: 「共 N 筆，這裡顯示 M 筆，因為 X」. Until
	// 2026-08-25 the page could only say 「超過 N 個」, and a lower bound cannot say
	// 共 -- a reader cannot tell 21 from 2100 by it. The reason half was already
	// here in Truncated's copy; this is the count.
	//
	// It comes from count(*) OVER () inside the retrieval statement, so it is
	// computed by the same WHERE that produced the rows and cannot drift from
	// them. On the hybrid path it is bounded by the 50-per-leg candidate window
	// (45 documents indexed, so exact today, and the SQL names the trigger for
	// when it stops being); on the degraded lexical path it is exact.
	Total int64 `json:"total"`
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
	// `q` is required by public.yaml, and Query().Get cannot tell an absent
	// parameter from an empty one. The distinction is not pedantic: `q=` is a
	// blank search, which DISC-001 has an answer for (no results plus a prompt
	// for what to add), while no `q` at all is a malformed request and answering
	// it 200 made the handler looser than the contract every generated client
	// is built from (M1 audit, 2026-08-24).
	if !r.URL.Query().Has("q") {
		httpx.WriteError(w, http.StatusBadRequest, "query parameter q is required")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if msg := queryTooLong(q); msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
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
	// Same position and the same reason: a `limit` outside the schema is refused
	// whatever the query says.
	limit, err := parseLimit(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// DISC-001: blank/incomprehensible queries don't create search.
	if q == "" || !isComprehensible(q) {
		// They do create a funnel event, though. 01 §11.2's first segment is a
		// ratio of sessions that submitted an intent, and this path is where the
		// intents the platform could not parse end up — a single Han character
		// like 圖 is one of them. Dropping them made the denominator exclude
		// exactly the sessions 01 §12 is about ("內容涵蓋不到使用者的任務"), which
		// is the reading the M5 generation entry is waiting for, so segment 1 read
		// systematically high (M5 audit, 2026-08-25).
		//
		// result_count 0 / has_results false is the truth: nothing was retrieved.
		// No new attribute — the whitelist is the same five.
		h.Svc.Analytics.SearchPerformed(r.Context(), q, 0, filters.active())
		httpx.WriteJSON(w, http.StatusOK, searchResponse{
			Query:           q,
			Results:         []searchResult{},
			NoResults:       true,
			QuerySuggestion: noResultsSuggestion,
			// Nothing was retrieved, so nothing was cut. Zero rather than the
			// default cap: `limit` describes this page, and naming a cap that never
			// applied would be a number with no enforcement behind it (§2.2).
			Limit: 0,
		})
		return
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
		Limit:          limit,
		Truncated:      out.Truncated,
		Total:          out.Total,
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

// catalogResponse is 02:DISC-006's payload: a page of a list and nothing else.
//
// Four fields, and the ones missing are missing because they would have been
// constants. `query` would always be empty, `degraded` always false (no model
// call on this path), `no_results` would mean 「the catalogue is empty」 here and
// 「nothing was close enough」 there — one name for two facts. A response that
// carries a field it can never fill is 設計系統 §2.9's failure at the contract
// layer, not a convenience.
type catalogResponse struct {
	Results []searchResult `json:"results"`
	// Limit is named because it is enforced (§2.2), and Truncated plus Total are
	// §4.3's 「共 N 筆，這裡顯示 M 筆」. Total is exact on this path always: there
	// is no candidate window to make it quietly become a lower bound, which is
	// the ceiling the search route has to declare and this one does not.
	Limit     int32 `json:"limit"`
	Total     int64 `json:"total"`
	Truncated bool  `json:"truncated"`
}

// BrowseCatalog handles GET /api/skills/catalog.
//
// 02:DISC-006. The home page asked every visitor to describe a task before it
// showed them anything, so a reader who did not already know what was in the
// catalogue had to guess a sentence to find out — 義務 1.2「快到第一個判斷」
// cannot be met by a page whose first judgement requires a correct guess. It is
// also the exit 資訊架構 IA-5 records as missing: when a search finds nothing,
// 「what IS here」 had no address at all.
//
// Anonymous, like search: DISC-001 serves the catalogue to anyone, the scope is
// fixed to catalogue workspaces inside the SQL (CORE-006), and no parameter can
// widen it.
func (h *Handler) BrowseCatalog(w http.ResponseWriter, r *http.Request) {
	// Parsed before anything is retrieved, and refused rather than ignored, for
	// the reason PublicSearch states: answering a filtered request with an
	// unfiltered page presents the whole catalogue as the filtered subset.
	filters, err := parseFilters(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	hits, total, err := h.Svc.Browse(r.Context(), limit, filters)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "catalog read failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, catalogResponse{
		Results:   hits,
		Limit:     limit,
		Total:     total,
		Truncated: total > int64(len(hits)),
	})
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
//
// Both sentences are in the interface language for the reason given at
// noResultsSuggestion: they are rendered verbatim. Only the interpolated terms
// are in whatever script the query and the document happened to share.
func templateMatchReason(name, summary, query string) string {
	if terms := overlapTerms(query, name+" "+summary); len(terms) > 0 {
		return "查詢與文件共同出現：" + strings.Join(terms, "、") + "。"
	}
	// Deliberately not "以語意找到的結果": there is no lexical evidence to show,
	// so this states what ordered the row and stops short of claiming it fits.
	return "沒有共同的關鍵字,這一列是以語意相似度靠近你的任務描述而排進來的,未必真的合用。"
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

// isComprehensible does a minimal check that the query carries at least two
// letters or digits (DISC-001: incomprehensible queries don't create search).
//
// The rule has to bite, because a query that gets past it is not free:
// PublicSearch answers it by buying an embedding from the gateway and writing a
// search_performed funnel event. "!!", "、、" and two spaces are not searches.
//
// unicode.IsLetter rather than an ASCII-plus-0x4E00 range: the range spelled out
// by hand covered Latin, digits and CJK ideographs, and silently excluded kana,
// Cyrillic, Greek, Thai and every abugida - all of which are queries this catalog
// is supposed to be able to receive. It was previously masked by an
// `|| utf8.RuneCountInString(q) >= 2` clause that let everything of length two
// through, which made the character rule dead code: replacing the whole body
// with `return len(q) >= 2` kept 316 tests green (M1 audit, 2026-08-24).
//
// One letter, not two. The first version of this fix demanded two and refused
// `C#`, `C++`, `F#` and `R` - single-letter-plus-symbol names, which are among
// the likeliest queries a Skill catalog receives. The rune floor is what still
// rejects a bare `1` or a single ideograph; the letter is what separates a name
// from punctuation. `a.` gets through, and costs one embedding.
// maxQueryRunes caps what may be called a query at all. Shared by both search
// routes: DISC-001 是對「搜尋」這個行為說的，不是對某一條路由說的，and until
// 2026-08-29 only the public route had it — so a 2 MB `q` went straight into
// websearch_to_tsquery on the workspace route.
const maxQueryRunes = 2000

// queryTooLong returns the 400 message for an over-long query, or "".
func queryTooLong(q string) string {
	if utf8.RuneCountInString(q) > maxQueryRunes {
		return "query parameter q must be at most 2000 characters"
	}
	return ""
}

func isComprehensible(q string) bool {
	// Bytes that are not text are not a query, and this is the gate both
	// routes already pass through, so it is the one place that has to say so.
	//
	// Ranging over a malformed string yields utf8.RuneError, which is neither a
	// letter nor a digit — but only for the bad bytes. `0xa7` followed by `A`
	// counted as two runes with a letter among them and went through, straight
	// into websearch_to_tsquery, where PostgreSQL answered SQLSTATE 22021 and
	// the handler turned that into a 500 on an endpoint that needs no login.
	//
	// Percent-decoding produces raw bytes and Go strings do not validate them,
	// so this is reachable from any client: a pasted Big5 fragment, a browser
	// with a legacy encoding, one crafted request. In clean mode it was worse
	// than a 500 (04 丙-112).
	if !utf8.ValidString(q) {
		return false
	}
	// A NUL is valid UTF-8, so the gate above waves it through — and PostgreSQL
	// cannot hold one in a text value at all, so `a\x00b` reached
	// websearch_to_tsquery and came back as the same 500 on the same
	// unauthenticated endpoint (04 丙-119, found by a monkey pass 2026-09-01,
	// the day after 丙-112 closed the invalid-UTF-8 half).
	//
	// Measured, because the two layers behave differently and only one of them
	// was doing its job: the deployment SURVIVED — that is 丙-112's second layer,
	// the exec mode with no server-side named statements — but the request after
	// the hostile one answered 500 as well before it recovered. One bad request,
	// two broken responses, and the second lands on somebody else.
	//
	// Every C0 control is refused, not just NUL. NUL is the one PostgreSQL
	// rejects outright, and the rest — a bare CR, a form feed, an escape
	// character — are not something a person typing a task description produces
	// either. \t \n \r are the exception: they arrive from a paste, they are
	// ordinary whitespace to the tokenizer, and refusing them would turn a
	// two-line paste into 「看不懂的查詢」.
	for _, r := range q {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
		if r == 0x7f {
			return false
		}
	}
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return utf8.RuneCountInString(q) >= 2
		}
	}
	return false
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

	// The same three rules the public route applies, because they are rules
	// about a query and not about a route (DISC-001 空白或無法理解的查詢不得建立
	// 搜尋). What differs is only the answer: this route has no funnel segment to
	// record and no model call to protect, so an unusable query is a 400 rather
	// than the public route's no-results state.
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "query parameter q is required")
		return
	}
	if msg := queryTooLong(q); msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	if !isComprehensible(q) {
		httpx.WriteError(w, http.StatusBadRequest,
			"query parameter q must contain at least two characters, one of them a letter or a digit")
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
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
			SkillID: pgconv.UUIDString(row.SkillID),
			Name:    row.Name,
			Summary: row.Summary,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": hits})
}
