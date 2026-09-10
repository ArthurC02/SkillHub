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

type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

const (
	reasonSourceModel    = "model"
	reasonSourceTemplate = "template"
)

const MaxCosineDistance = 0.75

const noResultsSuggestion = "試著補充三件事:你想完成的任務、你手上已經有的輸入," +
	"以及你預期得到的輸出。"

type searchResult struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`

	SummarySource string `json:"summary_source"`

	Rank     *float64 `json:"rank"`
	RankNote string   `json:"rank_note,omitempty"`
	Tier     labelled `json:"tier"`

	Category          labelled      `json:"category"`
	Risk              searchRisk    `json:"risk"`
	Dependencies      []string      `json:"dependencies"`
	Compat            compatibility `json:"compatibility"`
	VerifiedAt        string        `json:"verified_at,omitempty"`
	MatchReason       string        `json:"match_reason,omitempty"`
	MatchReasonSource string        `json:"match_reason_source,omitempty"`

	unranked bool
}

type searchRisk struct {
	ScanStatus string `json:"scan_status"`

	Level    string `json:"level"`
	Warnings int    `json:"warnings"`

	Disclosures []disclosure `json:"disclosures"`

	Note string `json:"note"`
}

const (
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

func resultFacets(r *searchResult, tier string, category, categorySource *string, tagsJSON, scanJSON []byte, verifiedAt pgtype.Timestamptz, compat compatibility) {
	r.Tier = tierLabel(Tier(tier))
	r.Category = categoryLabel(category, categorySource)
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

func measuredCompat(capability, runtime, image string, measuredAt pgtype.Timestamptz) compatibility {
	return compatibility{
		Capability:   axis(capabilityWords, capability),
		Runtime:      axis(runtimeWords, runtime),
		RuntimeImage: image,
		MeasuredAt:   timeString(measuredAt),
	}
}

func dependencyTags(tagsJSON []byte) []string {
	var t llmclient.SkillTags
	if len(tagsJSON) == 0 || json.Unmarshal(tagsJSON, &t) != nil || t.Dependencies == nil {
		return []string{}
	}
	return t.Dependencies
}

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

	case len(out.Disclosures) > 0:
		out.Level = riskLevelDisclosed
	default:
		out.Level = riskLevelNone
	}
	return out
}

type searchFilters struct {
	HasScript     *bool
	SpecValidated *bool

	AgentRuntime *string

	CurationTier *string

	Category *string
}

func (f searchFilters) active() bool {
	return f.HasScript != nil || f.SpecValidated != nil || f.AgentRuntime != nil ||
		f.CurationTier != nil || f.Category != nil
}

var curationTierValues = map[string]bool{
	string(TierCurated): true, string(TierIndexed): true,
}

var agentRuntimeValues = map[string]bool{
	"native": true, "transpiled": true, "failed": true, "unverified": true,
}

var categoryValues = map[string]bool{
	string(CategoryDocuments): true, string(CategoryWriting): true, string(CategoryData): true,
}

var unavailableFilters = map[string]string{
	"mcp": "是否需要 MCP 沒有任何來源資料:靜態掃描與 manifest 都沒有這項訊號,遠端 MCP 也不在 MVP 首發。",
}

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

type searchResponse struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`

	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degraded_reason,omitempty"`

	PartialIndex bool `json:"partial_index"`

	Limit     int32 `json:"limit"`
	Truncated bool  `json:"truncated"`

	Total int64 `json:"total"`

	NoResults bool `json:"no_results"`

	QuerySuggestion string `json:"query_suggestion,omitempty"`

	FilteredOut bool `json:"filtered_out"`
}

func (h *Handler) PublicSearch(w http.ResponseWriter, r *http.Request) {

	if !r.URL.Query().Has("q") {
		httpx.WriteError(w, http.StatusBadRequest, "query parameter q is required")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if msg := queryTooLong(q); msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}

	purpose := r.URL.Query().Get("purpose")
	if purpose != "" && purpose != "reference" {
		httpx.WriteError(w, http.StatusBadRequest, `query parameter purpose must be "reference"`)
		return
	}
	silent := purpose == "reference"

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

	if q == "" || !isComprehensible(q) {

		if !silent {
			h.Svc.Analytics.SearchPerformed(r.Context(), q, 0, filters.active())
		}
		httpx.WriteJSON(w, http.StatusOK, searchResponse{
			Query:           q,
			Results:         []searchResult{},
			NoResults:       true,
			QuerySuggestion: noResultsSuggestion,

			Limit: 0,
		})
		return
	}

	out, err := h.Svc.Search(r.Context(), q, limit, filters, silent)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "搜尋失敗，這不是你的輸入造成的，稍後再試一次")
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

	if len(out.Hits) == 0 && !out.FilteredOut {
		resp.NoResults = true
		resp.QuerySuggestion = noResultsSuggestion
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type catalogResponse struct {
	Results []searchResult `json:"results"`

	Limit     int32 `json:"limit"`
	Total     int64 `json:"total"`
	Truncated bool  `json:"truncated"`
}

func (h *Handler) BrowseCatalog(w http.ResponseWriter, r *http.Request) {

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
		httpx.WriteError(w, http.StatusInternalServerError, "目錄讀取失敗，稍後再試一次")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, catalogResponse{
		Results:   hits,
		Limit:     limit,
		Total:     total,
		Truncated: total > int64(len(hits)),
	})
}

func anyUnranked(hits []searchResult) bool {
	for _, h := range hits {
		if h.unranked {
			return true
		}
	}
	return false
}

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

func templateMatchReason(name, summary, query string) string {
	if terms := overlapTerms(query, name+" "+summary); len(terms) > 0 {
		return "查詢與文件共同出現：" + strings.Join(terms, "、") + "。"
	}

	return "沒有共同的關鍵字,這一列是以語意相似度靠近你的任務描述而排進來的,未必真的合用。"
}

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

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "from": true, "into": true, "you": true, "your": true,
	"can": true, "are": true, "use": true, "using": true, "how": true,
}

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

const maxQueryRunes = 2000

func queryTooLong(q string) string {
	if utf8.RuneCountInString(q) > maxQueryRunes {
		return "搜尋文字最多 2000 字"
	}
	return ""
}

func isComprehensible(q string) bool {

	if !utf8.ValidString(q) {
		return false
	}

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
		httpx.WriteError(w, http.StatusInternalServerError, "搜尋失敗，這不是你的輸入造成的，稍後再試一次")
		return
	}

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
