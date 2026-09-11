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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":           true,
		"versions_retained": res.VersionsRetained,
		"note":              deletionNote,
	})
}

const deletionNote = "已從你的工作區、清單與搜尋移除；版本快照維持凍結，這次刪除不會移除它們；" +
	"Fork 引用的共用套件物件不受影響"

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

var categoryValues = map[string]bool{
	"documents": true, "writing": true, "data": true, "unassigned": true,
}

func (h *Handler) SetCategory(w http.ResponseWriter, r *http.Request) {
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
		Category string `json:"category"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a category")
		return
	}
	if !categoryValues[body.Category] {
		httpx.WriteError(w, http.StatusBadRequest,
			`category must be "documents", "writing", "data" or "unassigned"`)
		return
	}

	var category *string
	if body.Category != "unassigned" {
		category = &body.Category
	}

	skill, err := h.Svc.SetCategory(r.Context(), ws, skillID, category)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "set category failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSkillResponse(skill))
}

type skillVersionResponse struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	CreatedAt     string `json:"created_at"`
}

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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}

	const listSkillsLimit = 100
	catalogs, err := h.Svc.catalogWorkspaceIDs(r.Context(), h.Svc.Pool)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	rows, err := gen.New(h.Svc.Pool).ListSkills(r.Context(), gen.ListSkillsParams{
		WorkspaceID: ws.ID, CatalogWorkspaceIds: catalogs, RowLimit: listSkillsLimit + 1, RowOffset: 0,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	truncated := len(rows) > listSkillsLimit

	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalMatches
	}
	if truncated {
		rows = rows[:listSkillsLimit]
	}

	if h.Svc.SkillRisks == nil || h.Svc.CatalogSkillRisks == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	ids := make([]pgtype.UUID, 0, len(rows))

	ancestors := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Skill.ID)
		if row.InheritedFromSkillID.Valid {
			ancestors = append(ancestors, row.InheritedFromSkillID)
		}
	}
	risks, err := h.Svc.SkillRisks(r.Context(), ws.ID, ids)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}
	inherited, err := h.Svc.CatalogSkillRisks(r.Context(), ancestors)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list failed")
		return
	}

	out := make([]ownSkillResponse, 0, len(rows))
	for _, row := range rows {
		risk := risks[pgconv.UUIDString(row.Skill.ID)]
		if row.InheritedFromSkillID.Valid {
			risk = inherited[pgconv.UUIDString(row.InheritedFromSkillID)]
		}
		out = append(out, ownSkillResponse{
			skillResponse: toSkillResponse(row.Skill),
			Risk:          risk,
			Verification:  verificationOf(row),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skills":    out,
		"limit":     listSkillsLimit,
		"truncated": truncated,
		"total":     total,
	})
}

type ownSkillResponse struct {
	skillResponse

	Risk         json.RawMessage   `json:"risk"`
	Verification skillVerification `json:"verification"`
}

type skillVerification struct {
	Value     string  `json:"value"`
	Label     string  `json:"label"`
	Note      string  `json:"note"`
	ScannedAt *string `json:"scanned_at"`
}

func verificationOf(row gen.ListSkillsRow) skillVerification {
	switch {
	case !row.VerifiedAt.Valid:
		return skillVerification{
			Value: "not_applicable",
			Label: "不適用",
			Note:  "這個 Skill 還沒有任何版本,沒有可掃描的內容。",
		}
	case row.InheritedFromSkillID.Valid:

		at := row.InheritedVerifiedAt.Time.UTC().Format(time.RFC3339)
		return skillVerification{
			Value: "scanned",
			Label: "已掃描（來源）",
			Note: "這個版本是 Fork 進來的複本,內容雜湊與來源「" + row.InheritedFromName +
				"」相同,所以沿用來源匯入時的靜態掃描結果。時間是來源掃描的時間,不是 Fork 的時間;" +
				"相容性與試跑結果不沿用,那些量的是內容在某個環境下的行為。",
			ScannedAt: &at,
		}
	case !row.VerifiedSourceID.Valid:

		return skillVerification{
			Value: "not_measured",
			Label: "未測量",
			Note: "這個版本是 Fork 進來的複本,而它的來源現在無法提供掃描結果" +
				"(已下架、不在公開目錄,或內容已經不同),平台也沒有在你的工作區重跑。",
		}
	default:
		at := row.VerifiedAt.Time.UTC().Format(time.RFC3339)
		return skillVerification{
			Value:     "scanned",
			Label:     "已掃描",
			Note:      "匯入這個版本時做過靜態掃描,不執行套件內任何程式碼;逐項結果在 Skill 頁面。",
			ScannedAt: &at,
		}
	}
}
