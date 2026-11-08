package mcptools

import (
	"errors"
	"net/http"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxBodyBytes = 16 << 10

type toolView struct {
	ID           string  `json:"id"`
	Reference    string  `json:"reference"`
	Server       string  `json:"server"`
	Tool         string  `json:"tool"`
	Purpose      string  `json:"purpose"`
	Parameters   string  `json:"parameters"`
	ServerURL    string  `json:"server_url"`
	Reason       string  `json:"reason"`
	CreatedAt    string  `json:"created_at"`
	RetiredAt    *string `json:"retired_at,omitempty"`
	RetireReason *string `json:"retire_reason,omitempty"`
}

func viewOf(t Tool) toolView {
	v := toolView{
		ID: t.ID, Reference: t.Reference(), Server: t.Server, Tool: t.Tool, Purpose: t.Purpose, Parameters: t.Parameters,
		ServerURL: t.ServerURL, Reason: t.Reason, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
	}
	if t.RetiredAt != nil {
		at := t.RetiredAt.UTC().Format(time.RFC3339)
		v.RetiredAt, v.RetireReason = &at, &t.RetireReason
	}
	return v
}

func (s *Service) ListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "MCP tool lookup failed")
		return
	}
	views := make([]toolView, 0, len(tools))
	for _, t := range tools {
		views = append(views, viewOf(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tools": views})
}

func (s *Service) AddTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Server     string `json:"server"`
		Tool       string `json:"tool"`
		Purpose    string `json:"purpose"`
		Parameters string `json:"parameters"`
		ServerURL  string `json:"server_url"`
		Note       string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxBodyBytes, &body, "body must be JSON with `server`, `tool`, `purpose`, `parameters`, `server_url` and `note`") {
		return
	}
	operator, _ := identity.SessionUser(r.Context())
	t, err := s.Add(r.Context(), operator.ID, Draft(body))
	writeTool(w, http.StatusCreated, t, err)
}

func (s *Service) RetireTool(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathUUID(w, r, "id", ErrNotFound.Error())
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxBodyBytes, &body, "body must be JSON with `note`") {
		return
	}
	operator, _ := identity.SessionUser(r.Context())
	t, err := s.Retire(r.Context(), operator.ID, id, body.Note)
	writeTool(w, http.StatusOK, t, err)
}

func writeTool(w http.ResponseWriter, code int, t Tool, err error) {
	switch {
	case err == nil:
		httpx.WriteJSON(w, code, viewOf(t))
	case errors.Is(err, ErrShape):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrConflict):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "MCP tool change failed")
	}
}
