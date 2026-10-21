package evalsettings

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxSettingsBodyBytes = 16 << 10

type Handler struct {
	Svc   *Service
	Actor func(*http.Request) (pgtype.UUID, bool)
}

type judgePanelView struct {
	Enabled     bool   `json:"enabled"`
	Reason      string `json:"reason,omitempty"`
	SetByUserID string `json:"set_by_user_id,omitempty"`
	SetAt       string `json:"set_at,omitempty"`
}

type settingsView struct {
	JudgePanel judgePanelView `json:"judge_panel"`
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Svc.Settings(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOfSettings(settings))
}

func (h *Handler) SetJudgePanel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool  `json:"enabled"`
		Note    string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, maxSettingsBodyBytes, &body, "body must be JSON") {
		return
	}
	if body.Enabled == nil {
		httpx.WriteError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	operator, ok := h.Actor(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	settings, err := h.Svc.SetJudgePanel(r.Context(), operator, *body.Enabled, body.Note)
	if errors.Is(err, ErrSettingsReasonRequired) {
		httpx.WriteError(w, http.StatusBadRequest, "note is required and at most 1000 characters")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "settings update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOfSettings(settings))
}

func viewOfSettings(s Settings) settingsView {
	panel := judgePanelView{Enabled: s.JudgePanel}
	if s.Changed {
		panel.Reason = s.Reason
		panel.SetAt = s.SetAt.UTC().Format(time.RFC3339)
		if s.SetBy.Valid {
			panel.SetByUserID = pgconv.UUIDString(s.SetBy)
		}
	}
	return settingsView{JudgePanel: panel}
}
