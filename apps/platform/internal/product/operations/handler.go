package operations

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const (
	maxNoteBytes        = 1000
	maxRequestBodyBytes = 16 << 10
)

type Handler struct {
	Svc   *Service
	Actor func(*http.Request) (pgtype.UUID, bool)
}

type agentView struct {
	Name                string   `json:"name"`
	Purpose             string   `json:"purpose"`
	ModelRole           string   `json:"model_role"`
	DailySpendCapMicros int64    `json:"daily_spend_cap_usd_micros"`
	Tools               []string `json:"tools"`
	Actions             []string `json:"actions"`
	Enabled             bool     `json:"enabled"`
	OwnerUserID         string   `json:"owner_user_id,omitempty"`
}

type brakeView struct {
	Reason          string `json:"reason"`
	EngagedAt       string `json:"engaged_at"`
	EngagedByUserID string `json:"engaged_by_user_id,omitempty"`
}

type agentsResponse struct {
	Agents []agentView `json:"agents"`
	Brake  *brakeView  `json:"brake,omitempty"`
}

type enabledRequest struct {
	Enabled *bool  `json:"enabled"`
	Note    string `json:"note"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	agents, brake, err := h.Svc.Agents(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent lookup failed")
		return
	}
	response := agentsResponse{Agents: make([]agentView, len(agents))}
	for i, a := range agents {
		response.Agents[i] = view(a)
	}
	if brake != nil {
		b := brakeBody(*brake)
		response.Brake = &b
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	var body enabledRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		httpx.WriteError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	note, ok := requireNote(w, body.Note)
	if !ok {
		return
	}
	operator, ok := h.operator(w, r)
	if !ok {
		return
	}
	flip := h.Svc.Disable
	if *body.Enabled {
		flip = h.Svc.Enable
	}
	updated, err := flip(r.Context(), r.PathValue("name"), operator, note)
	if errors.Is(err, ErrUnknownAgent) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view(updated))
}

func (h *Handler) EngageBrake(w http.ResponseWriter, r *http.Request) {
	note, operator, ok := h.noteFromOperator(w, r)
	if !ok {
		return
	}
	brake, err := h.Svc.EngageBrake(r.Context(), operator, note)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent brake failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, brakeBody(brake))
}

func (h *Handler) ReleaseBrake(w http.ResponseWriter, r *http.Request) {
	note, operator, ok := h.noteFromOperator(w, r)
	if !ok {
		return
	}
	if err := h.Svc.ReleaseBrake(r.Context(), operator, note); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "agent brake release failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) noteFromOperator(w http.ResponseWriter, r *http.Request) (string, pgtype.UUID, bool) {
	var body struct {
		Note string `json:"note"`
	}
	if !decode(w, r, &body) {
		return "", pgtype.UUID{}, false
	}
	note, ok := requireNote(w, body.Note)
	if !ok {
		return "", pgtype.UUID{}, false
	}
	operator, ok := h.operator(w, r)
	return note, operator, ok
}

func decode(w http.ResponseWriter, r *http.Request, body any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)).Decode(body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a note")
		return false
	}
	return true
}

func requireNote(w http.ResponseWriter, raw string) (string, bool) {
	note := strings.TrimSpace(raw)
	if note == "" {
		httpx.WriteError(w, http.StatusBadRequest, "note is required: say why, so the next operator can find out")
		return "", false
	}
	if len(note) > maxNoteBytes {
		httpx.WriteError(w, http.StatusBadRequest, "note is too long")
		return "", false
	}
	return note, true
}

func (h *Handler) operator(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	id, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
	}
	return id, ok
}

func view(a Agent) agentView {
	v := agentView{
		Name: a.Name, Purpose: a.Purpose, ModelRole: a.ModelRole, DailySpendCapMicros: a.DailySpendCapMicros,
		Tools: nonNil(a.Tools), Actions: nonNil(a.Actions), Enabled: a.Enabled,
	}
	if a.OwnerID.Valid {
		v.OwnerUserID = pgconv.UUIDString(a.OwnerID)
	}
	return v
}

func brakeBody(b Brake) brakeView {
	return brakeView{
		Reason: b.Reason, EngagedAt: b.EngagedAt.UTC().Format(time.RFC3339),
		EngagedByUserID: pgconv.UUIDString(b.EngagedBy),
	}
}
