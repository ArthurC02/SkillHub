package modelbudget

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// A Handler exposes the settings to operator routes. Actor reports who is
// calling; the composition root supplies it, and a caller it cannot name is
// answered with the same 404 an unauthorised caller gets.
type Handler struct {
	Svc   *Service
	Actor func(*http.Request) (pgtype.UUID, bool)
}

type budgetRequest struct {
	Seconds int    `json:"seconds"`
	Reason  string `json:"reason"`
}

type budgetView struct {
	Kind           string  `json:"kind"`
	Seconds        *int    `json:"seconds"`
	DefaultSeconds int     `json:"default_seconds"`
	MinSeconds     int     `json:"min_seconds"`
	MaxSeconds     int     `json:"max_seconds"`
	Reason         *string `json:"reason"`
	SetAt          *string `json:"set_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.actor(w, r); !ok {
		return
	}
	settings, endpoints, err := h.Svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "model budget lookup failed")
		return
	}
	views := make([]budgetView, 0, len(endpoints))
	for i, e := range endpoints {
		views = append(views, viewOf(e, settings[i]))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"budgets": views})
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	body, ok := decode(w, r)
	if !ok {
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	setting, err := h.Svc.Set(r.Context(), kind, body.Seconds, body.Reason, actor)
	if err != nil {
		h.writeRefusal(w, err, "model budget could not be set")
		return
	}
	e, _ := h.Svc.endpoint(kind)
	httpx.WriteJSON(w, http.StatusOK, viewOf(e, setting))
}

func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	body, ok := decode(w, r)
	if !ok {
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Clear(r.Context(), kind, body.Reason, actor); err != nil {
		h.writeRefusal(w, err, "model budget could not be cleared")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	if h.Actor == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return pgtype.UUID{}, false
	}
	actor, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return pgtype.UUID{}, false
	}
	return actor, true
}

func (h *Handler) writeRefusal(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrUnknownKind):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest,
			"reason is required: changing how long the platform waits for a model is an "+
				"operator action nobody can explain later otherwise")
	case errors.Is(err, ErrOutOfRange):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, fallback)
	}
}

func decode(w http.ResponseWriter, r *http.Request) (budgetRequest, bool) {
	var body budgetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a reason")
		return budgetRequest{}, false
	}
	return body, true
}

func viewOf(e Endpoint, s Setting) budgetView {
	view := budgetView{
		Kind:           e.Kind,
		DefaultSeconds: e.Ceiling(),
		MinSeconds:     MinSeconds,
		MaxSeconds:     e.Ceiling(),
	}
	if s.Seconds > 0 {
		seconds, reason, setAt := s.Seconds, s.Reason, s.SetAt.UTC().Format(time.RFC3339)
		view.Seconds, view.Reason, view.SetAt = &seconds, &reason, &setAt
	}
	return view
}
