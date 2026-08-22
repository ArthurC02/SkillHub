package registry

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// Handler exposes registry endpoints (contracts/openapi/public.yaml). All
// routes require a session; the workspace comes from it (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, false
	}
	return ws, true
}

type skillResponse struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Both of these were already on the row `ListSkills` selects and were dropped
	// here. They are two of the four locks that refuse a download, and the owner's
	// own list is where they matter most: `unknown` is what a user's own import
	// carries by default (0027), so a skill they cannot take away looked exactly
	// like one they could until they reached the packaging screen.
	Redistribution      string  `json:"redistribution"`
	AccessRestriction   *string `json:"access_restriction"`
	ForkedFromSkillID   *string `json:"forked_from_skill_id,omitempty"`
	ForkedFromVersionID *string `json:"forked_from_version_id,omitempty"`
}

func toSkillResponse(s gen.Skill) skillResponse {
	out := skillResponse{
		SkillID:           pgconv.UUIDString(s.ID),
		Name:              s.Name,
		Redistribution:    s.Redistribution,
		AccessRestriction: s.AccessRestriction,
	}
	if s.Summary != nil {
		out.Summary = *s.Summary
	}
	if s.ForkedFromSkillID.Valid {
		v := pgconv.UUIDString(s.ForkedFromSkillID)
		out.ForkedFromSkillID = &v
	}
	if s.ForkedFromVersionID.Valid {
		v := pgconv.UUIDString(s.ForkedFromVersionID)
		out.ForkedFromVersionID = &v
	}
	return out
}

// Fork handles POST /skills/{id}/fork.
func (h *Handler) Fork(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	fork, ver, err := h.Svc.Fork(r.Context(), ws, skillID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrNameTaken):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "fork failed")
		return
	}

	resp := struct {
		skillResponse
		VersionID     string `json:"version_id"`
		VersionNumber int32  `json:"version_number"`
	}{toSkillResponse(fork), pgconv.UUIDString(ver.ID), ver.VersionNumber}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// Delete handles DELETE /skills/{id} (WS-005).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	res, err := h.Svc.Delete(r.Context(), ws, skillID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	// 狀態回饋 (WS-005): say exactly what the deletion covers.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":           true,
		"versions_retained": res.VersionsRetained,
		"note": "skill removed from your workspace, lists, and search; " +
			"version snapshots are retained for the 30-day grace period, then purged; " +
			"shared package objects referenced by forks are unaffected",
	})
}

// Takedown handles POST /skills/{id}/takedown (INGEST-010). The caller must own
// the workspace the skill lives in; see Service.Takedown for why that is the
// whole authorization story in the MVP.
func (h *Handler) Takedown(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a reason")
		return
	}
	// A takedown nobody can explain later is not a moderation decision.
	if strings.TrimSpace(body.Reason) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "reason is required")
		return
	}

	skill, err := h.Svc.Takedown(r.Context(), ws, skillID, strings.TrimSpace(body.Reason))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrAlreadyTakenDown):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "takedown failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id":    pgconv.UUIDString(skill.ID),
		"takedown_at": skill.TakedownAt.Time.UTC().Format(time.RFC3339),
		"reason":      body.Reason,
		"note": "removed from search and from the fork path; the skill, its versions " +
			"and their sources are retained, and existing forks and past runs are unaffected",
	})
}

// skillVersionResponse mirrors the inline `version` object the skill detail
// serves, so the version history and the detail view name one version the same
// way.
type skillVersionResponse struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	CreatedAt     string `json:"created_at"`
}

// Versions handles GET /skills/{id}/versions (WS-001) — the version history the
// pre-run and packaging screens pick from.
//
// A skill outside the caller's workspace answers 200 with an empty list rather
// than 404: the scope comes from the session (iron rule 3), so a caller cannot
// tell the two apart anyway, and the screens that read this treat "no versions"
// as "nothing to pick" either way.
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	rows, err := gen.New(h.Svc.Pool).ListSkillVersions(r.Context(), gen.ListSkillVersionsParams{
		WorkspaceID: ws.ID, SkillID: skillID,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := make([]skillVersionResponse, 0, len(rows))
	for _, v := range rows {
		out = append(out, skillVersionResponse{
			VersionID:     pgconv.UUIDString(v.ID),
			VersionNumber: v.VersionNumber,
			ContentHash:   v.ContentHash,
			CreatedAt:     v.CreatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": out})
}

// Diff handles GET /skills/{id}/diff?from=&to= (WS-003).
func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID, fromID, toID pgtype.UUID
	if skillID.Scan(r.PathValue("id")) != nil ||
		fromID.Scan(r.URL.Query().Get("from")) != nil ||
		toID.Scan(r.URL.Query().Get("to")) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "id, from, and to must be version UUIDs")
		return
	}

	diffs, err := h.Svc.DiffVersions(r.Context(), ws, skillID, fromID, toID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "diff failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"files": diffs})
}

// List handles GET /skills — the caller's own skills (WS-004).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	// Ask for one more than the cap so the cap can be reported rather than
	// enforced silently. The limit was already here; what was missing is that a
	// workspace past it got a short list with nothing saying so, and a list that
	// is quietly incomplete reads as a complete answer.
	const listSkillsLimit = 100
	rows, err := gen.New(h.Svc.Pool).ListSkills(r.Context(), gen.ListSkillsParams{
		WorkspaceID: ws.ID, Limit: listSkillsLimit + 1, Offset: 0,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	truncated := len(rows) > listSkillsLimit
	if truncated {
		rows = rows[:listSkillsLimit]
	}
	out := make([]skillResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, toSkillResponse(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skills":    out,
		"limit":     listSkillsLimit,
		"truncated": truncated,
	})
}
