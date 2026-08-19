package sandbox

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// maxRequestBytes bounds a RunRequest. The prompt travels inline and dataset
// bytes do not, so a body larger than this is not a run the contract describes.
const maxRequestBytes = 1 << 20

// Server serves contracts/openapi/sandbox-provider.yaml. The control plane is
// its only client.
type Server struct {
	M *Manager
	// Token is the static provider token from securitySchemes.providerToken. It
	// authenticates the control plane and carries no workspace, user or run
	// scope, so it can never widen what a request may reach (iron rule 3).
	Token string
	// Metrics is this node's Prometheus registry (O11Y-001/003). Nil leaves
	// /metrics off the route table entirely rather than serving an empty page.
	Metrics *prometheus.Registry
}

// Routes returns the route table. It is one reviewable list on purpose: every
// route is behind the same bearer check, and there is no unauthenticated path.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /capability", s.auth(s.capability))
	mux.HandleFunc("POST /runs", s.auth(s.createRun))
	mux.HandleFunc("GET /runs", s.auth(s.listRuns))
	mux.HandleFunc("GET /runs/{provider_run_id}", s.auth(s.getRun))
	mux.HandleFunc("DELETE /runs/{provider_run_id}", s.auth(s.destroyRun))
	mux.HandleFunc("POST /runs/{provider_run_id}/cancel", s.auth(s.cancelRun))
	// Behind the same bearer check as everything else: this table has no
	// unauthenticated path, and an exposition endpoint is not where to make the
	// first exception (NFR-005).
	if s.Metrics != nil {
		metrics := MetricsHandler(s.Metrics)
		mux.HandleFunc("GET /metrics", s.auth(metrics.ServeHTTP))
	}
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Constant time: a token comparison that returns early leaks the token
		// one byte at a time to anyone willing to measure.
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or invalid provider token")
			return
		}
		next(w, r)
	}
}

func (s *Server) capability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.M.Capability(r.Context()))
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be a RunRequest object")
		return
	}

	run, created, err := s.M.Create(r.Context(), req)
	var capErr *RunError
	switch {
	case errors.As(err, &capErr):
		writeJSON(w, http.StatusUnprocessableEntity, capErr)
		return
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict,
			"this (run_id, attempt) was dispatched before with different content")
		return
	case errors.Is(err, ErrNoSlot):
		writeError(w, http.StatusTooManyRequests, "no free run slot right now")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, run)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	// `active=false` is answered 400, not with an empty list: this endpoint
	// never had finished runs to give, and an empty list would read as "none".
	if v := r.URL.Query().Get("active"); v != "" && v != "true" {
		writeError(w, http.StatusBadRequest, "only active=true is served")
		return
	}
	writeJSON(w, http.StatusOK, s.M.List())
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.M.Get(r.PathValue("provider_run_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.M.Cancel(r.Context(), r.PathValue("provider_run_id"))
	if err != nil {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) destroyRun(w http.ResponseWriter, r *http.Request) {
	if err := s.M.Destroy(r.Context(), r.PathValue("provider_run_id")); err != nil {
		writeError(w, http.StatusInternalServerError, "resources could not be released")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the contract's Error shape. The string is masked and safe
// to log; machine-readable classification is RunError.class, not this.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
