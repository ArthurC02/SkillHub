package run

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
)

// Handler exposes the run endpoints (contracts/openapi/public.yaml). Every route
// requires a session and takes its workspace from it (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (gen.Workspace, gen.User, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return gen.Workspace{}, gen.User{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return gen.Workspace{}, gen.User{}, false
	}
	return ws, user, true
}

type runResponse struct {
	RunID             string           `json:"run_id"`
	Status            string           `json:"status"`
	StatusReason      string           `json:"status_reason,omitempty"`
	SkillVersionID    string           `json:"skill_version_id"`
	TestCaseSnapshot  string           `json:"test_case_snapshot_id"`
	Provider          string           `json:"provider"`
	FailureClass      string           `json:"failure_class,omitempty"`
	CleanupStatus     string           `json:"cleanup_status"`
	CancelRequestedAt string           `json:"cancel_requested_at,omitempty"`
	CreatedAt         string           `json:"created_at"`
	StartedAt         string           `json:"started_at,omitempty"`
	FinishedAt        string           `json:"finished_at,omitempty"`
	Transitions       []transitionView `json:"transitions,omitempty"`
	Attempts          []attemptView    `json:"attempts,omitempty"`
}

type transitionView struct {
	From       string `json:"from_status,omitempty"`
	To         string `json:"to_status"`
	Reason     string `json:"reason,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

// attemptView is the RUN-003 mapping as the API exposes it: the platform ids are
// the identity, the provider's id is an attribute of one attempt.
type attemptView struct {
	RunAttemptID  string `json:"run_attempt_id"`
	AttemptNumber int32  `json:"attempt_number"`
	Provider      string `json:"provider"`
	ProviderRunID string `json:"provider_run_id,omitempty"`
	ErrorClass    string `json:"error_class,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

func toRunResponse(run gen.Run) runResponse {
	return runResponse{
		RunID:             uuidString(run.ID),
		Status:            string(run.Status),
		StatusReason:      deref(run.StatusReason),
		SkillVersionID:    uuidString(run.SkillVersionID),
		TestCaseSnapshot:  uuidString(run.TestCaseSnapshotID),
		Provider:          run.Provider,
		FailureClass:      deref(run.FailureClass),
		CleanupStatus:     string(run.CleanupStatus),
		CancelRequestedAt: rfc3339(run.CancelRequestedAt),
		CreatedAt:         rfc3339(run.CreatedAt),
		StartedAt:         rfc3339(run.StartedAt),
		FinishedAt:        rfc3339(run.FinishedAt),
	}
}

// Create handles POST /skills/{id}/runs (RUN-001).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	var body struct {
		VersionID  string `json:"version_id"`
		TestCaseID string `json:"test_case_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with version_id and test_case_id")
		return
	}
	var versionID, testCaseID pgtype.UUID
	if versionID.Scan(body.VersionID) != nil || testCaseID.Scan(body.TestCaseID) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "version_id and test_case_id must be UUIDs")
		return
	}

	run, err := h.Svc.Create(r.Context(), CreateParams{
		WorkspaceID: ws.ID, Actor: user.ID,
		SkillID: skillID, VersionID: versionID, TestCaseID: testCaseID,
	})
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	// RUN-005: no configured provider can run this, and the message says which
	// requirement each one failed. 422, not 500: the request is well-formed and the
	// platform is working, it simply cannot be satisfied.
	if errors.Is(err, ErrNoCompatibleProvider) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run creation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRunResponse(run))
}

// Get handles GET /runs/{id} (RUN-002: current status and how it got there).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var runID pgtype.UUID
	if err := runID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	run, err := h.Svc.Get(r.Context(), ws.ID, runID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run lookup failed")
		return
	}

	resp := toRunResponse(run)
	transitions, err := h.Svc.History(r.Context(), ws.ID, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run history lookup failed")
		return
	}
	for _, t := range transitions {
		v := transitionView{To: string(t.ToStatus), Reason: deref(t.Reason), OccurredAt: rfc3339(t.OccurredAt)}
		if t.FromStatus != nil {
			v.From = string(*t.FromStatus)
		}
		resp.Transitions = append(resp.Transitions, v)
	}
	attempts, err := h.Svc.Attempts(r.Context(), ws.ID, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run attempt lookup failed")
		return
	}
	for _, a := range attempts {
		resp.Attempts = append(resp.Attempts, attemptView{
			RunAttemptID:  uuidString(a.ID),
			AttemptNumber: a.AttemptNumber,
			Provider:      a.Provider,
			ProviderRunID: deref(a.ProviderRunID),
			ErrorClass:    deref(a.ErrorClass),
			ErrorMessage:  deref(a.ErrorMessage),
			StartedAt:     rfc3339(a.StartedAt),
			FinishedAt:    rfc3339(a.FinishedAt),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// Cancel handles POST /runs/{id}/cancel (RUN-004). It records intent and says so;
// the run keeps its current status until the workload is actually down (RUN-006).
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var runID pgtype.UUID
	if err := runID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	run, err := h.Svc.RequestCancel(r.Context(), ws.ID, runID, user.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrRunFinished):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "cancel failed")
		return
	}

	resp := struct {
		runResponse
		Note string `json:"note"`
	}{toRunResponse(run), "cancellation requested; the run keeps its current status " +
		"until the workload has actually stopped"}
	httpx.WriteJSON(w, http.StatusAccepted, resp)
}

func rfc3339(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
