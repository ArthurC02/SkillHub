package run

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxSettingsBodyBytes = 16 << 10

type SettingsHandler struct {
	Svc   *Service
	Actor func(*http.Request) (pgtype.UUID, bool)
}

type continuationRoundsView struct {
	Rounds      int    `json:"rounds"`
	Min         int    `json:"min"`
	Max         int    `json:"max"`
	Reason      string `json:"reason,omitempty"`
	SetByUserID string `json:"set_by_user_id,omitempty"`
	SetAt       string `json:"set_at,omitempty"`
}

type runSettingsView struct {
	ContinuationRounds continuationRoundsView `json:"continuation_rounds"`
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Svc.Settings(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOfSettings(settings))
}

func (h *SettingsHandler) SetContinuationRounds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rounds *int   `json:"rounds"`
		Note   string `json:"note"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON")
		return
	}
	if body.Rounds == nil {
		httpx.WriteError(w, http.StatusBadRequest, "rounds is required")
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	settings, err := h.Svc.SetContinuationRounds(r.Context(), operator, *body.Rounds, body.Note)
	switch {
	case errors.Is(err, ErrSettingsReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest, "note is required and at most 1000 bytes")
	case errors.Is(err, ErrRoundsOutOfRange):
		httpx.WriteError(w, http.StatusBadRequest, "rounds must be between 1 and 10")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "settings update failed")
	default:
		httpx.WriteJSON(w, http.StatusOK, viewOfSettings(settings))
	}
}

func viewOfSettings(s Settings) runSettingsView {
	rounds := continuationRoundsView{
		Rounds: s.ContinuationRounds, Min: MinContinuationRounds, Max: MaxContinuationRounds,
	}
	if s.Changed {
		rounds.Reason = s.Reason
		rounds.SetAt = s.SetAt.UTC().Format(time.RFC3339)
		if s.SetBy.Valid {
			rounds.SetByUserID = pgconv.UUIDString(s.SetBy)
		}
	}
	return runSettingsView{ContinuationRounds: rounds}
}
