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

// maxBatchBytes bounds one ingestion request. A sandbox is untrusted input, so
// its body is bounded before it is parsed, not after.
const maxBatchBytes = 4 << 20

// maxBatchEvents bounds how many events one request may carry. The collector
// sends what it has accumulated since its last push; anything above this is a
// producer that has stopped respecting its own budget.
const maxBatchEvents = 1000

// Handler serves both faces of the trace: the machine-to-machine ingestion
// endpoint the execution plane posts to, and the two user-facing read modes.
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

// Ingest handles POST /internal/trace/{token}.
//
// It is authenticated by the token in the path and by nothing else: there is no
// session, because the caller is a sandbox provider, and no provider bearer
// token, because that credential is deployment-wide and this endpoint must not
// accept a run's events on the authority of a credential that covers every run.
//
// Iron rule 2 is intact in both directions: the execution plane does not touch
// the database, it posts JSON to the control plane, which decides everything.
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
	// 202, not 201: the events are accepted, and the producer must not read this
	// as "all of them were new" - some were duplicates and the report says so.
	httpx.WriteJSON(w, http.StatusAccepted, report)
}

// Get handles GET /runs/{id}/trace?mode=general|advanced (TRACE-006/007).
//
// One route with a mode rather than two routes: both answers are the same rows
// read two ways, and a client toggling between them should not have to know two
// URLs.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	// Workspace from the session, never from the request (iron rule 3).
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

	// `after` is judged before the mode is, and that is the fix rather than an
	// accident of ordering. It used to be parsed inside the `advanced` arm, so
	// `?mode=general&after=-5` was answered with page one of the general view
	// while `?mode=advanced&after=-5` was a 400 -- one endpoint giving two
	// different answers to the same schema violation, which is the shape the
	// limit/offset work removed from the paging endpoints on 2026-08-24. A cursor
	// the caller cannot have meant is refused whichever view they asked for
	// (corrected 2026-08-25).
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
