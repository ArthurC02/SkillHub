package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxFeedbackMessage = 2000

type Handler struct {
	Svc      *Service
	Identity *identity.Service

	FeedbackRetention time.Duration
}

func (h *Handler) DownloadStartedOn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.Svc.Enabled() {
			var artifactID pgtype.UUID
			if artifactID.Scan(r.PathValue("artifactId")) == nil {
				var workspace pgtype.UUID
				if user, ok := identity.SessionUser(r.Context()); ok {
					if ws, err := h.Identity.PersonalWorkspace(r.Context(), user); err == nil {
						workspace = ws.ID
					}
				}

				h.Svc.DownloadStarted(r.Context(), workspace, artifactID, "")
			}
		}
		next(w, r)
	}
}

func (h *Handler) Feedback(w http.ResponseWriter, r *http.Request) {
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
		Kind     string `json:"kind"`
		Message  string `json:"message"`
		PagePath string `json:"page_path"`
		RunID    string `json:"run_id"`
		BuildID  string `json:"build_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "內容必須是 JSON，且包含 kind 與 message")
		return
	}
	if body.Kind != "blocking_issue" && body.Kind != "need_signal" {
		httpx.WriteError(w, http.StatusBadRequest, "kind 必須是 blocking_issue 或 need_signal")
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len([]rune(message)) > maxFeedbackMessage {
		httpx.WriteError(w, http.StatusBadRequest, "message 不能空白，且最多 2000 字")
		return
	}

	p := gen.InsertFeedbackReportParams{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		Kind:        body.Kind,
		Message:     message,
	}

	if path := strings.TrimSpace(body.PagePath); strings.HasPrefix(path, "/") &&
		!strings.ContainsAny(path, "?#") && len(path) <= 512 {
		p.PagePath = &path
	}

	if build := strings.TrimSpace(body.BuildID); build != "" && len(build) <= 64 {
		p.BuildID = &build
	}

	var runID pgtype.UUID
	if body.RunID != "" && runID.Scan(body.RunID) == nil {
		if h.Svc.RunBelongsToWorkspace == nil {
			httpx.WriteError(w, http.StatusInternalServerError, "feedback run lookup unavailable")
			return
		}
		mine, err := h.Svc.RunBelongsToWorkspace(r.Context(), ws.ID, runID)
		if err == nil && mine {
			p.RunID = runID
		}
	}

	if err := gen.New(h.Svc.Pool).InsertFeedbackReport(r.Context(), p); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "回報沒有記錄成功，可以再送一次")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) PurgeExpiredFeedback(ctx context.Context, retention time.Duration) (int64, error) {
	if s == nil || s.Pool == nil {
		return 0, errors.New("feedback purge requires a database pool")
	}
	if retention <= 0 {
		return 0, errors.New("feedback purge requires a positive retention period")
	}
	return gen.New(s.Pool).DeleteExpiredFeedbackReports(ctx,
		pgtype.Timestamptz{Time: s.now().Add(-retention), Valid: true})
}
