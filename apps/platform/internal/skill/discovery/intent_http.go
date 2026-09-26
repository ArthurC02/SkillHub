package catalog

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxCorrectedSearchBytes = 128 << 10

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
	interpretation := SearchInterpretation{Status: "corrected", Intent: body.Intent, Keywords: body.Keywords, Filters: body.Filters}
	if _, err := interpretation.validate(body.Query, false); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := int32(20)
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
	out, err := h.Svc.searchInterpreted(r.Context(), body.Query, limit, interpretation, false)
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
	}
	if len(out.Hits) == 0 && !out.FilteredOut {
		resp.NoResults = true
		resp.QuerySuggestion = noResultsSuggestion
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
