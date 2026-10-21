package eval

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
)

const maxGraderBodyBytes = 32 << 10

type GraderHandler struct {
	Svc      *graders.Service
	Identity *identity.Service
}

type graderView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Scope        string `json:"scope"`
	VersionID    string `json:"version_id"`
	Version      int32  `json:"version"`
	Instructions string `json:"instructions"`
	RevisedAt    string `json:"revised_at"`
}

func viewOfGrader(g graders.Grader) graderView {
	scope := "workspace"
	if g.Platform {
		scope = "platform"
	}
	return graderView{
		ID: g.ID, Name: g.Name, Scope: scope, VersionID: g.VersionID, Version: g.Version,
		Instructions: g.Instructions, RevisedAt: g.RevisedAt.UTC().Format(time.RFC3339),
	}
}

func (h *GraderHandler) ListGraders(w http.ResponseWriter, r *http.Request) {
	ws, ok := sessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	catalog, err := h.Svc.List(r.Context(), ws.ID)
	writeGraders(w, catalog, err)
}

func (h *GraderHandler) CreateGrader(w http.ResponseWriter, r *http.Request) {
	ws, ok := sessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	h.create(w, r, ws.ID)
}

func (h *GraderHandler) ReviseGrader(w http.ResponseWriter, r *http.Request) {
	ws, ok := sessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	h.revise(w, r, ws.ID)
}

func (h *GraderHandler) ListPlatformGraders(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.Svc.ListPlatform(r.Context())
	writeGraders(w, catalog, err)
}

func (h *GraderHandler) CreatePlatformGrader(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, pgtype.UUID{})
}

func (h *GraderHandler) RevisePlatformGrader(w http.ResponseWriter, r *http.Request) {
	h.revise(w, r, pgtype.UUID{})
}

func (h *GraderHandler) create(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID) {
	draft, ok := decodeGraderDraft(w, r)
	if !ok {
		return
	}
	user, _ := identity.SessionUser(r.Context())
	g, err := h.Svc.Create(r.Context(), workspaceID, user.ID, draft)
	writeGrader(w, g, err)
}

func (h *GraderHandler) revise(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID) {
	graderID, ok := pathUUID(w, r)
	if !ok {
		return
	}
	draft, ok := decodeGraderDraft(w, r)
	if !ok {
		return
	}
	user, _ := identity.SessionUser(r.Context())
	g, err := h.Svc.Revise(r.Context(), workspaceID, graderID, user.ID, draft)
	writeGrader(w, g, err)
}

func decodeGraderDraft(w http.ResponseWriter, r *http.Request) (graders.Draft, bool) {
	var body struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
		Reason       string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, maxGraderBodyBytes, &body, "body must be JSON with `instructions` and `reason`") {
		return graders.Draft{}, false
	}
	return graders.Draft(body), true
}

func writeGraders(w http.ResponseWriter, catalog []graders.Grader, err error) {
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "grader lookup failed")
		return
	}
	views := make([]graderView, len(catalog))
	for i, g := range catalog {
		views[i] = viewOfGrader(g)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"graders": views})
}

func writeGrader(w http.ResponseWriter, g graders.Grader, err error) {
	switch {
	case errors.Is(err, graders.ErrShape):
		httpx.WriteError(w, http.StatusBadRequest, reasonOf(err, graders.ErrShape))
	case errors.Is(err, graders.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, reasonOf(err, graders.ErrConflict))
	case errors.Is(err, graders.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "grader not found")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "the grader could not be saved")
	default:
		httpx.WriteJSON(w, http.StatusCreated, viewOfGrader(g))
	}
}

func reasonOf(err, sentinel error) string {
	return strings.TrimPrefix(err.Error(), sentinel.Error()+": ")
}
