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
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	// SkillID and TestCaseID are not columns of `runs`; they are filled by
	// fillLinkage. They are served because their consumers cannot derive them and
	// should not have to be told them out of band: applying suggestions posts to
	// /skills/{id}/versions/from-suggestions, and a re-run needs the editable test
	// case rather than the snapshot it was frozen into. Neither is permission to do
	// either thing (TEST-009 still stands).
	SkillID           string           `json:"skill_id"`
	SkillVersionID    string           `json:"skill_version_id"`
	TestCaseSnapshot  string           `json:"test_case_snapshot_id"`
	TestCaseID        string           `json:"test_case_id,omitempty"`
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

// fillLinkage adds the two ids the runs row does not carry. One lookup for all
// three run responses rather than each handler assembling its own from whatever it
// happens to have in scope — a `skill_id` that is present on one response and
// absent on another is a field no client can rely on.
func (h *Handler) fillLinkage(
	w http.ResponseWriter, r *http.Request, ws, runID pgtype.UUID, resp *runResponse,
) bool {
	link, err := h.Svc.Linkage(r.Context(), ws, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run lookup failed")
		return false
	}
	resp.SkillID = uuidString(link.SkillID)
	resp.TestCaseID = uuidString(link.TestCaseID)
	return true
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
		// The summary the user confirmed (02:TEST-005). Absent or stale is a 422
		// below; it is never inferred from a previous run.
		ConfirmedSummaryHash string `json:"confirmed_summary_hash"`
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
		ConfirmedSummaryHash: body.ConfirmedSummaryHash,
	})
	// SEC-012: the fleet is held for a P1. 503 and not 422, which is the whole rest
	// of this list: every refusal below says the request may not proceed while the
	// platform is fine, and this one says the platform is not. It carries no
	// incident detail — the person who cannot start a run is not the person the
	// incident is about.
	if errors.Is(err, ErrDispatchHalted) {
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	// SEC-002 gate B. 422, like the capability refusal: the request is well formed
	// and the platform is working, it simply may not proceed until the user has
	// agreed to the current permissions.
	if errors.Is(err, ErrPermissionsNotConfirmed) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// RUN-005: no configured provider can run this, and the message says which
	// requirement each one failed. 422, not 500: the request is well-formed and the
	// platform is working, it simply cannot be satisfied.
	if errors.Is(err, ErrNoCompatibleProvider) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// SEC-002 gate B's other two blocking conditions (internal/run/gateb.go): a
	// blocking or unperformable static scan, and the workspace concurrency ceiling.
	// Same 422 as the two above, for the same reason - nothing about the request is
	// malformed, it simply may not proceed. The messages say which, and what to do.
	// Same 422 for the 0023 licensing hold: the request is fine, the platform is
	// fine, and this content may not be copied into a sandbox until the review
	// concludes.
	//
	// PDM-010's allowance joins them (ADR-028 決策 2): out of runs for this window
	// is not a malformed request either, and 422 keeps the whole of gate B on one
	// status code. The message carries the reset time, because "come back later"
	// without a time is the version of this screen nobody can act on.
	if errors.Is(err, ErrScanBlocked) || errors.Is(err, ErrRunLimitReached) ||
		errors.Is(err, ErrAccessRestricted) || errors.Is(err, ErrQuotaExceeded) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run creation failed")
		return
	}
	resp := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, run.ID, &resp) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// Quota handles GET /me/quota (PDM-010). It reports the counters
// POST /skills/{id}/runs enforces, and computes nothing of its own — a display
// with its own arithmetic is free to disagree with the rule, and the direction it
// would disagree in is the generous one (04 乙-2).
//
// Mounted only where an allowance is actually enforced (see apiserver.NewRouter),
// so a deployment with no allowance answers 404 here rather than serving numbers
// nothing applies.
func (h *Handler) Quota(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	state, enforced, err := h.Svc.QuotaFor(r.Context(), ws.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "quota lookup failed")
		return
	}
	if !enforced {
		httpx.WriteError(w, http.StatusNotFound, "no run allowance is configured")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state.View())
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
	if !h.fillLinkage(w, r, ws.ID, runID, &resp) {
		return
	}
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

	body := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, runID, &body) {
		return
	}
	resp := struct {
		runResponse
		Note string `json:"note"`
	}{body, "cancellation requested; the run keeps its current status " +
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
