package catalog

import (
	"net/http"
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

func (h *Handler) FindSkillsForGovernance(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "q is required")
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(q); err != nil {
		skillID = pgtype.UUID{}
	}
	found, err := registry.SkillsForGovernance(r.Context(), h.Svc.Pool, skillID, q)
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
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"skills": views})
}
