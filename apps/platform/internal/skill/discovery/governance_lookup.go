package catalog

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type governanceView struct {
	SkillID           string  `json:"skill_id"`
	WorkspaceID       string  `json:"workspace_id"`
	Name              string  `json:"name"`
	AccessRestriction *string `json:"access_restriction"`
	Redistribution    string  `json:"redistribution"`
	TakedownAt        *string `json:"takedown_at"`
	TakedownReason    *string `json:"takedown_reason"`
}

type governanceSearchResponse struct {
	Skills     []governanceView `json:"skills"`
	Total      int64            `json:"total"`
	NextOffset *int64           `json:"next_offset,omitempty"`
}

func (h *Handler) FindSkillsForGovernance(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "q is required")
		return
	}
	var offset int64
	if r.URL.Query().Has("offset") {
		var err error
		offset, err = strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)
		if err != nil || offset < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "offset must be a nonnegative integer")
			return
		}
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(q); err != nil {
		skillID = pgtype.UUID{}
	}
	found, total, err := registry.SkillsForGovernance(r.Context(), h.Svc.Pool, skillID, q, int32(offset))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "skill lookup failed")
		return
	}
	views := make([]governanceView, 0, len(found))
	for _, g := range found {
		view := governanceView{
			SkillID: pgconv.UUIDString(g.ID), WorkspaceID: pgconv.UUIDString(g.WorkspaceID), Name: g.Name,
			AccessRestriction: g.AccessRestriction, Redistribution: g.Redistribution, TakedownReason: g.TakedownReason,
		}
		if g.TakedownAt.Valid {
			at := pgconv.RFC3339(g.TakedownAt)
			view.TakedownAt = &at
		}
		views = append(views, view)
	}
	response := governanceSearchResponse{Skills: views, Total: total}
	if len(views) > 0 && offset+int64(len(views)) < total {
		next := offset + int64(len(views))
		response.NextOffset = &next
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}
