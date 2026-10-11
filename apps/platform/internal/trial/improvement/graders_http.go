package eval

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxGraderBodyBytes = 32 << 10

type GraderHandler struct {
	Svc      *GraderService
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

func viewOfGrader(g Grader) graderView {
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
	graders, err := h.Svc.List(r.Context(), ws.ID)
	writeGraders(w, graders, err)
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
	graders, err := h.Svc.ListPlatform(r.Context())
	writeGraders(w, graders, err)
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

func decodeGraderDraft(w http.ResponseWriter, r *http.Request) (GraderDraft, bool) {
	var body struct {
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
		Reason       string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxGraderBodyBytes)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with `instructions` and `reason`")
		return GraderDraft{}, false
	}
	return GraderDraft(body), true
}

func writeGraders(w http.ResponseWriter, graders []Grader, err error) {
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "grader lookup failed")
		return
	}
	views := make([]graderView, len(graders))
	for i, g := range graders {
		views[i] = viewOfGrader(g)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"graders": views})
}

func writeGrader(w http.ResponseWriter, g Grader, err error) {
	switch {
	case errors.Is(err, ErrGraderShape):
		httpx.WriteError(w, http.StatusBadRequest, reasonOf(err, ErrGraderShape))
	case errors.Is(err, ErrGraderConflict):
		httpx.WriteError(w, http.StatusConflict, reasonOf(err, ErrGraderConflict))
	case errors.Is(err, ErrNotFound):
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
