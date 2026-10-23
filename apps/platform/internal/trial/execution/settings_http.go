package run

import (
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

type modelBudgetView struct {
	USD         float64 `json:"usd"`
	DefaultUSD  float64 `json:"default_usd"`
	MinUSD      float64 `json:"min_usd"`
	MaxUSD      float64 `json:"max_usd"`
	Reason      string  `json:"reason,omitempty"`
	SetByUserID string  `json:"set_by_user_id,omitempty"`
	SetAt       string  `json:"set_at,omitempty"`
}

type runSettingsView struct {
	ContinuationRounds continuationRoundsView `json:"continuation_rounds"`
	ModelBudget        modelBudgetView        `json:"model_budget"`
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	h.writeSettings(w, r)
}

func (h *SettingsHandler) writeSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Svc.Settings(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings lookup failed")
		return
	}
	budget, err := runBudgetIn(r.Context(), h.Svc.queries(), h.Svc.Deployment)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOfSettings(settings, budget))
}

func (h *SettingsHandler) SetRunModelBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		USD  *float64 `json:"usd"`
		Note string   `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
		return
	}
	if body.USD == nil {
		httpx.WriteError(w, http.StatusBadRequest, "usd is required")
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	_, err := setRunBudget(r.Context(), h.Svc, operator, *body.USD, body.Note)
	h.answerSettingsChange(w, r, err)
}

func (h *SettingsHandler) ClearRunModelBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	_, err := clearRunBudget(r.Context(), h.Svc, operator, body.Note)
	h.answerSettingsChange(w, r, err)
}

func (h *SettingsHandler) answerSettingsChange(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrSettingsReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest, "note is required and at most 1000 characters")
	case errors.Is(err, ErrRunBudgetOutOfRange):
		httpx.WriteError(w, http.StatusBadRequest, "usd must be between 0.05 and 5")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "settings update failed")
	default:
		h.writeSettings(w, r)
	}
}

func (h *SettingsHandler) SetContinuationRounds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rounds *int   `json:"rounds"`
		Note   string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
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
	_, err := h.Svc.SetContinuationRounds(r.Context(), operator, *body.Rounds, body.Note)
	if errors.Is(err, ErrRoundsOutOfRange) {
		httpx.WriteError(w, http.StatusBadRequest, "rounds must be between 1 and 10")
		return
	}
	h.answerSettingsChange(w, r, err)
}

func viewOfSettings(s Settings, budget RunBudgetSetting) runSettingsView {
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
	return runSettingsView{ContinuationRounds: rounds, ModelBudget: viewOfRunBudget(budget)}
}

func viewOfRunBudget(b RunBudgetSetting) modelBudgetView {
	view := modelBudgetView{USD: b.USD, DefaultUSD: b.DefaultUSD, MinUSD: MinRunBudgetUSD, MaxUSD: MaxRunBudgetUSD}
	if b.Overridden {
		view.Reason = b.Reason
		view.SetAt = b.SetAt.UTC().Format(time.RFC3339)
		if b.SetBy.Valid {
			view.SetByUserID = pgconv.UUIDString(b.SetBy)
		}
	}
	return view
}
