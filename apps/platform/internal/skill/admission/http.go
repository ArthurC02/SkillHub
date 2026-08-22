package ingest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// Handler exposes skill import endpoints (contracts/openapi/public.yaml).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

// UploadResult is public.yaml's UploadResult. Exported because
// POST /skills/{id}/versions/from-suggestions answers with this shape plus two
// fields (contract: `allOf`), and a second copy of it in internal/eval is exactly
// how two endpoints start describing the same version differently.
type UploadResult struct {
	SkillID       string                       `json:"skill_id"`
	VersionID     string                       `json:"version_id"`
	VersionNumber int32                        `json:"version_number"`
	ContentHash   string                       `json:"content_hash"`
	Duplicate     bool                         `json:"duplicate"`
	Findings      skillpkg.CategorizedFindings `json:"findings"`
}

// NewUploadResult renders one stored version the way every creation path does.
func NewUploadResult(res Result) UploadResult {
	return UploadResult{
		SkillID:       pgconv.UUIDString(res.Skill.ID),
		VersionID:     pgconv.UUIDString(res.Version.ID),
		VersionNumber: res.Version.VersionNumber,
		ContentHash:   res.Version.ContentHash,
		Duplicate:     res.Duplicate,
		Findings:      res.Report.Categorize(),
	}
}

// Upload handles POST /skills/import/upload. Wrap with RequireSession; the
// workspace is derived from the session, never from the client (iron rule 3).
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, skillpkg.MaxZipBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "package exceeds the upload size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	res, err := h.Svc.UploadZip(r.Context(), ws, data)
	h.respond(w, res, err)
}

// SaveVersion handles POST /skills/{id}/versions (WS-002). Wrap with
// RequireSession.
func (h *Handler) SaveVersion(w http.ResponseWriter, r *http.Request) {
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

	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrSkillNotFound.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, skillpkg.MaxZipBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "package exceeds the upload size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	res, err := h.Svc.SaveVersion(r.Context(), ws, skillID, data)
	if errors.Is(err, ErrSkillNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	h.respond(w, res, err)
}

// ImportURL handles POST /skills/import/url. Wrap with RequireSession.
func (h *Handler) ImportURL(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || body.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a non-empty url")
		return
	}

	res, err := h.Svc.ImportURL(r.Context(), ws, body.URL)
	h.respond(w, res, err)
}

func (h *Handler) respond(w http.ResponseWriter, res Result, err error) {
	if errors.Is(err, skillpkg.ErrBadArchive) || errors.Is(err, ErrFetch) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "import failed")
		return
	}

	if res.Report.Blocked {
		// SKILL-001/INGEST-008: failure result carries error/warning/info
		// findings as separate lists, not one undifferentiated feed.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, res.Report.Categorize())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, NewUploadResult(res))
}
