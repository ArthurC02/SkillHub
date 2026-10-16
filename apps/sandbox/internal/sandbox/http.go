package sandbox

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const maxRequestBytes = 1 << 20

type Server struct {
	M *Manager

	Token string

	Metrics *prometheus.Registry
}

type route struct {
	pattern string
	handler http.HandlerFunc
}

func (s *Server) routeTable() []route {
	table := []route{
		{"GET /capability", s.capability},
		{"POST /runs", s.createRun},
		{"GET /runs", s.listRuns},
		{"GET /runs/{provider_run_id}", s.getRun},
		{"DELETE /runs/{provider_run_id}", s.destroyRun},
		{"POST /runs/{provider_run_id}/cancel", s.cancelRun},
	}
	if s.Metrics != nil {
		table = append(table, route{"GET /metrics", MetricsHandler(s.Metrics).ServeHTTP})
	}
	return table
}

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range s.routeTable() {
		mux.HandleFunc(r.pattern, s.auth(r.handler))
	}
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Constant-time compare so a mismatch takes the same time regardless
		// of where the first differing byte falls.
		if !bearer || s.Token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
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
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "body must be a RunRequest object")
		return
	}

	run, created, err := s.M.Create(r.Context(), req)
	capErr, isCapErr := errors.AsType[*RunError](err)
	switch {
	case isCapErr && capErr.Retryable:
		writeJSON(w, http.StatusServiceUnavailable, capErr)
		return
	case isCapErr:
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

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
