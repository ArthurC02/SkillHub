package trace

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxBatchBytes = 4 << 20

const maxBatchEvents = 1000

type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	grant, err := h.Svc.Signer.Verify(token, time.Now())
	switch {
	case errors.Is(err, ErrTokenExpired):
		metrics.TraceIngestRejected.WithLabelValues("expired").Inc()
		httpx.WriteError(w, http.StatusUnauthorized, "trace ingestion token has expired")
		return
	case err != nil:
		metrics.TraceIngestRejected.WithLabelValues("bad_token").Inc()
		httpx.WriteError(w, http.StatusUnauthorized, "invalid trace ingestion token")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBatchBytes))
	if err != nil {
		metrics.TraceIngestRejected.WithLabelValues("too_large").Inc()
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "trace batch is too large")
		return
	}
	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		metrics.TraceIngestRejected.WithLabelValues("malformed").Inc()
		httpx.WriteError(w, http.StatusBadRequest, "body must be a JSON array of trace events")
		return
	}
	if len(events) > maxBatchEvents {
		metrics.TraceIngestRejected.WithLabelValues("too_many").Inc()
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too many events in one batch")
		return
	}

	report, err := h.Svc.Ingest(r.Context(), grant, token, events)
	switch {
	case errors.Is(err, ErrNotFound):
		metrics.TraceIngestRejected.WithLabelValues("unknown_run").Inc()
		httpx.WriteError(w, http.StatusNotFound, "no such run")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "trace ingestion failed")
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, report)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
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
	var runID pgtype.UUID
	if err := runID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "run not found")
		return
	}

	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid trace cursor")
			return
		}
	}

	var payload any
	switch r.URL.Query().Get("mode") {
	case "", "general":
		payload, err = h.Svc.General(r.Context(), ws.ID, runID)
	case "advanced":
		payload, err = h.Svc.Advanced(r.Context(), ws.ID, runID, after)
	default:
		httpx.WriteError(w, http.StatusBadRequest, "invalid trace mode")
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "run not found")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "trace lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payload)
}
