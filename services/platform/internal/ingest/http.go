package ingest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// Handler exposes skill import endpoints (contracts/openapi/public.yaml).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

type uploadResponse struct {
	SkillID       string             `json:"skill_id"`
	VersionID     string             `json:"version_id"`
	VersionNumber int32              `json:"version_number"`
	ContentHash   string             `json:"content_hash"`
	Duplicate     bool               `json:"duplicate"`
	Findings      []skillpkg.Finding `json:"findings"`
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

	r.Body = http.MaxBytesReader(w, r.Body, MaxZipBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "package exceeds the upload size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	res, err := h.Svc.UploadZip(r.Context(), ws, data)
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
	if errors.Is(err, ErrBadArchive) || errors.Is(err, ErrFetch) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "import failed")
		return
	}

	findings := res.Report.Findings
	if findings == nil {
		findings = []skillpkg.Finding{}
	}
	if res.Report.Blocked {
		// SKILL-001/INGEST-008: failure result carries the reasons.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"findings": findings})
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, uploadResponse{
		SkillID:       uuidString(res.Skill.ID),
		VersionID:     uuidString(res.Version.ID),
		VersionNumber: res.Version.VersionNumber,
		ContentHash:   res.Version.ContentHash,
		Duplicate:     res.Duplicate,
		Findings:      findings,
	})
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
