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
	Credits        int64  `json:"credits"`
	DefaultCredits int64  `json:"default_credits"`
	MinCredits     int64  `json:"min_credits"`
	MaxCredits     int64  `json:"max_credits"`
	Reason         string `json:"reason,omitempty"`
	SetByUserID    string `json:"set_by_user_id,omitempty"`
	SetAt          string `json:"set_at,omitempty"`
}

type tokensPerMinuteView struct {
	Limit        int    `json:"limit"`
	DefaultLimit int    `json:"default_limit"`
	Min          int    `json:"min"`
	Max          int    `json:"max"`
	Reason       string `json:"reason,omitempty"`
	SetByUserID  string `json:"set_by_user_id,omitempty"`
	SetAt        string `json:"set_at,omitempty"`
}

type runSettingsView struct {
	ContinuationRounds continuationRoundsView `json:"continuation_rounds"`
	ModelBudget        modelBudgetView        `json:"model_budget"`
	TokensPerMinute    tokensPerMinuteView    `json:"tokens_per_minute"`
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
	priced, ok := viewOfRunBudget(h.Svc.Ledger, budget)
	if !ok {
		httpx.WriteError(w, http.StatusServiceUnavailable, "the run spend ceiling cannot be priced in credits")
		return
	}
	rate, err := runRateLimitIn(r.Context(), h.Svc.queries(), h.Svc.Deployment)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings lookup failed")
		return
	}
	view := viewOfSettings(settings, priced)
	view.TokensPerMinute = viewOfRunRateLimit(rate)
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *SettingsHandler) SetRunModelBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Credits *int64 `json:"credits"`
		Note    string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
		return
	}
	if body.Credits == nil {
		httpx.WriteError(w, http.StatusBadRequest, "credits is required")
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	_, err := setRunBudget(r.Context(), h.Svc, operator, *body.Credits, body.Note)
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

func (h *SettingsHandler) SetRunTokensPerMinute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit *int   `json:"limit"`
		Note  string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
		return
	}
	if body.Limit == nil {
		httpx.WriteError(w, http.StatusBadRequest, "limit is required")
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	h.answerSettingsChange(w, r, setRunRateLimit(r.Context(), h.Svc, operator, *body.Limit, body.Note))
}

func (h *SettingsHandler) ClearRunTokensPerMinute(w http.ResponseWriter, r *http.Request) {
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
	h.answerSettingsChange(w, r, clearRunRateLimit(r.Context(), h.Svc, operator, body.Note))
}

func (h *SettingsHandler) answerSettingsChange(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrSettingsReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest, "note is required and at most 1000 characters")
	case errors.Is(err, ErrRunBudgetOutOfRange):
		httpx.WriteError(w, http.StatusBadRequest, "credits must be within the reported min_credits and max_credits")
	case errors.Is(err, ErrRunRateLimitOutOfRange):
		httpx.WriteError(w, http.StatusBadRequest, "limit must be between 10000 and 2000000")
	case errors.Is(err, ErrRunBudgetUnpriced):
		httpx.WriteError(w, http.StatusServiceUnavailable, "the run spend ceiling cannot be priced in credits")
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

func viewOfSettings(s Settings, budget modelBudgetView) runSettingsView {
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
	return runSettingsView{ContinuationRounds: rounds, ModelBudget: budget}
}

func viewOfRunBudget(l CreditLedger, b RunBudgetSetting) (modelBudgetView, bool) {
	band, ok := runBudgetBand(l)
	if !ok {
		return modelBudgetView{}, false
	}
	current, okCurrent := l.CreditsForUSD(b.USD)
	fallback, okDefault := l.CreditsForUSD(b.DefaultUSD)
	view := modelBudgetView{Credits: current, DefaultCredits: fallback, MinCredits: band.MinCredits, MaxCredits: band.MaxCredits}
	if b.Overridden {
		view.Reason = b.Reason
		view.SetAt = b.SetAt.UTC().Format(time.RFC3339)
		if b.SetBy.Valid {
			view.SetByUserID = pgconv.UUIDString(b.SetBy)
		}
	}
	return view, okCurrent && okDefault
}

func viewOfRunRateLimit(rate RunRateLimitSetting) tokensPerMinuteView {
	view := tokensPerMinuteView{Limit: rate.Limit, DefaultLimit: rate.DefaultLimit, Min: MinRunTPMLimit, Max: MaxRunTPMLimit}
	if rate.Overridden {
		view.Reason = rate.Reason
		view.SetAt = rate.SetAt.UTC().Format(time.RFC3339)
		if rate.SetBy.Valid {
			view.SetByUserID = pgconv.UUIDString(rate.SetBy)
		}
	}
	return view
}
