package publishing

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type bundleMemberView struct {
	SkillID       string `json:"skill_id"`
	VersionID     string `json:"version_id"`
	Name          string `json:"name"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
}

type bundleVersionView struct {
	Bundle      string             `json:"bundle"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	ContentHash string             `json:"content_hash"`
	CreatedAt   string             `json:"created_at"`
	Members     []bundleMemberView `json:"members"`
}

type memberChangeView struct {
	Name   string `json:"name"`
	Change string `json:"change"`
	From   int32  `json:"from,omitempty"`
	To     int32  `json:"to,omitempty"`
}

const (
	changeAdded    = "added"
	changeRemoved  = "removed"
	changeUpgraded = "changed"
)

func bundleView(v BundleVersion) bundleVersionView {
	members := make([]bundleMemberView, 0, len(v.Members))
	for _, m := range v.Members {
		members = append(members, bundleMemberView{
			SkillID: pgconv.UUIDString(m.SkillID), VersionID: pgconv.UUIDString(m.VersionID),
			Name: m.Name, VersionNumber: m.VersionNumber, ContentHash: m.ContentHash,
		})
	}
	return bundleVersionView{
		Bundle: v.Bundle, Version: v.Version, Description: v.Description,
		ContentHash: v.ContentHash, CreatedAt: timestamp(v.CreatedAt), Members: members,
	}
}

func memberChanges(newer, older BundleVersion) []memberChangeView {
	before := make(map[pgtype.UUID]BundleMember, len(older.Members))
	for _, m := range older.Members {
		before[m.SkillID] = m
	}
	changes := []memberChangeView{}
	for _, m := range newer.Members {
		previous, existed := before[m.SkillID]
		delete(before, m.SkillID)
		switch {
		case !existed:
			changes = append(changes, memberChangeView{Name: m.Name, Change: changeAdded, To: m.VersionNumber})
		case previous.VersionID != m.VersionID:
			changes = append(changes, memberChangeView{Name: m.Name, Change: changeUpgraded, From: previous.VersionNumber, To: m.VersionNumber})
		}
	}
	for _, m := range older.Members {
		if _, gone := before[m.SkillID]; gone {
			changes = append(changes, memberChangeView{Name: m.Name, Change: changeRemoved, From: m.VersionNumber})
		}
	}
	return changes
}

func (h *Handler) OwnBundles(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	versions, err := h.Svc.Bundles(r.Context(), ws)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "bundle lookup failed")
		return
	}
	out := make([]bundleVersionView, 0, len(versions))
	for _, v := range versions {
		out = append(out, bundleView(v))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bundles": out})
}

func (h *Handler) CreateBundleVersion(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req struct {
		Name             string   `json:"name"`
		Version          string   `json:"version"`
		Description      string   `json:"description"`
		MemberVersionIDs []string `json:"member_version_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the body must be a JSON object")
		return
	}
	in := BundleInput{Name: req.Name, Version: req.Version, Description: req.Description}
	for _, raw := range req.MemberVersionIDs {
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
		in.MemberVersionIDs = append(in.MemberVersionIDs, id)
	}
	created, err := h.Svc.CreateBundleVersion(r.Context(), ws, in)
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, bundleView(created))
}

func (h *Handler) ExportBundle(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	acquisition, err := h.Svc.ExportBundle(r.Context(), ws, r.PathValue("name"), r.URL.Query().Get("version"))
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, acquisitionViewOf(acquisition))
}

func (h *Handler) OwnBundlePublication(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	publication, found, err := h.Svc.OwnBundlePublication(r.Context(), ws, r.PathValue("name"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "publication lookup failed")
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "this Bundle has not been published")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}

func (h *Handler) PublishBundle(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req struct {
		Name           string `json:"name"`
		Version        string `json:"version"`
		RightsAttested bool   `json:"rights_attested"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the body must be a JSON object")
		return
	}
	publication, err := h.Svc.PublishBundle(r.Context(), ws, r.PathValue("name"), BundlePublishInput{
		Name: req.Name, Version: req.Version, RightsAttested: req.RightsAttested,
	})
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}

func (h *Handler) DelistBundle(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	publication, err := h.Svc.DelistBundle(r.Context(), ws, r.PathValue("name"))
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}
