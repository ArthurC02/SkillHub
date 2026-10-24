package catalog

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

const maxCorrectedSearchBytes = 128 << 10

const maxMatchReasonsBytes = 16 << 10

type matchReasonsRequest struct {
	Query    string   `json:"query"`
	SkillIDs []string `json:"skill_ids"`
}

type matchReasonView struct {
	SkillID string `json:"skill_id"`
	Reason  string `json:"reason"`
}

type matchReasonsResponse struct {
	Reasons []matchReasonView `json:"reasons"`
}

func (h *Handler) PublicMatchReasons(w http.ResponseWriter, r *http.Request) {
	var body matchReasonsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMatchReasonsBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "invalid match reasons request")
		return
	}
	query := strings.TrimSpace(body.Query)
	if query == "" || utf8.RuneCountInString(query) > limits.SearchMatchReasonsRequestQueryMaxLength {
		httpx.WriteError(w, http.StatusBadRequest, "query must contain between 1 and 2000 characters")
		return
	}
	ids, msg := distinctSkillIDs(body.SkillIDs)
	if msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	reasons, err := h.Svc.MatchReasonsFor(r.Context(), query, ids)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "推薦理由讀取失敗，結果不受影響")
		return
	}
	views := make([]matchReasonView, 0, len(reasons))
	for _, reason := range reasons {
		if reason.Reason != "" {
			views = append(views, matchReasonView(reason))
		}
	}
	httpx.WriteJSON(w, http.StatusOK, matchReasonsResponse{Reasons: views})
}

func distinctSkillIDs(raw []string) ([]pgtype.UUID, string) {
	if len(raw) < 1 || len(raw) > limits.SearchMatchReasonsRequestSkillIdsMaxItems {
		return nil, "skill_ids must list between 1 and 10 ids"
	}
	ids := make([]pgtype.UUID, 0, len(raw))
	seen := make(map[pgtype.UUID]bool, len(raw))
	for _, s := range raw {
		var id pgtype.UUID
		if err := id.Scan(s); err != nil {
			return nil, "skill_ids must be UUIDs"
		}
		if seen[id] {
			return nil, "skill_ids must not repeat"
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, ""
}

type correctedSearchRequest struct {
	Query    string             `json:"query"`
	Intent   map[string]*string `json:"intent"`
	Keywords []string           `json:"keywords"`
	Filters  map[string]string  `json:"filters"`
	Limit    json.RawMessage    `json:"limit,omitempty"`
}

func (h *Handler) CorrectedSearch(w http.ResponseWriter, r *http.Request) {
	var body correctedSearchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorrectedSearchBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid corrected search request")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, "expected one corrected search request")
		return
	}
	if strings.TrimSpace(body.Query) == "" || queryTooLong(body.Query) != "" {
		httpx.WriteError(w, http.StatusBadRequest, "query must contain between 1 and 2000 characters")
		return
	}
	interpretation := SearchInterpretation{Status: interpretationCorrected, Intent: body.Intent, Keywords: body.Keywords, Filters: body.Filters}
	if err := interpretation.validate(body.Query, correctedByUser); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := int32(defaultSearchLimit)
	if body.Limit != nil {
		if string(body.Limit) == "null" || json.Unmarshal(body.Limit, &limit) != nil {
			httpx.WriteError(w, http.StatusBadRequest, "limit must be an integer between 1 and 100")
			return
		}
	}
	if limit < 1 || limit > 100 {
		httpx.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}
	out, err := h.Svc.searchInterpreted(r.Context(), interpretedSearch{
		original: body.Query, limit: limit, interpretation: interpretation, purpose: searchByPerson,
	})
	h.writeSearchResult(w, body.Query, limit, out, err)
}

func (h *Handler) writeSearchResult(w http.ResponseWriter, query string, limit int32, out searchOutcome, err error) {
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "搜尋失敗，這不是你的輸入造成的，稍後再試一次")
		return
	}
	resp := searchResponse{
		Query: query, Results: out.Hits, Interpretation: out.Interpretation,
		Degraded: out.DegradedReason != "", DegradedReason: out.DegradedReason,
		PartialIndex: anyUnranked(out.Hits), FilteredOut: out.FilteredOut,
		Limit: limit, Truncated: out.Truncated, Total: out.Total,
		ReasonQuery: out.ReasonQuery,
	}
	if len(out.Hits) == 0 && !out.FilteredOut {
		resp.NoResults = true
		resp.QuerySuggestion = noResultsSuggestion
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
