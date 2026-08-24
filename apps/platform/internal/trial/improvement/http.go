package eval

// The evaluation read surface of contracts/openapi/public.yaml. Three routes, all
// workspace scoped from the session (iron rule 3), all answering 404 to anyone the
// run does not belong to — existence is itself private (WS-006).
//
// A run with no evaluation is 404 and not an empty body: 「未評估」 is a state of
// its own, and a blank body is exactly what a UI would render as a pass. An
// evaluation that ran and broke is a third state again, and it does have a body:
// `status: failed`.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// Handler exposes GET /runs/{id}/evaluation, its revision list, and the feedback
// PUT.
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, false
	}
	return ws, true
}

type evaluationView struct {
	EvaluationID          string            `json:"evaluation_id"`
	RunID                 string            `json:"run_id"`
	Status                string            `json:"status"`
	Overall               string            `json:"overall"`
	Summary               string            `json:"summary,omitempty"`
	CriterionResults      []CriterionResult `json:"criterion_results"`
	DeterministicFindings []Finding         `json:"deterministic_findings"`
	JudgeModel            string            `json:"judge_model"`
	JudgePromptVersion    string            `json:"judge_prompt_version"`
	RubricVersion         string            `json:"rubric_version,omitempty"`
	EvidenceComplete      bool              `json:"evidence_complete"`
	Cost                  costView          `json:"cost"`
	Feedback              *feedbackView     `json:"feedback,omitempty"`
	EvaluatedAt           string            `json:"evaluated_at"`
	SupersededAt          *string           `json:"superseded_at"`
}

// costView is what the judgement itself cost, and nothing else. It is never added
// to the run's cost: the two are spent by different workloads under different
// keys, and one combined number would inherit the weaker guarantee silently.
type costView struct {
	EvaluationUSD *float64 `json:"evaluation_usd"`
	Source        string   `json:"source"`
	Note          string   `json:"note"`
}

type feedbackView struct {
	Helpful     bool   `json:"helpful"`
	Comment     string `json:"comment,omitempty"`
	SubmittedAt string `json:"submitted_at"`
}

type revisionView struct {
	EvaluationID       string  `json:"evaluation_id"`
	JudgePromptVersion string  `json:"judge_prompt_version"`
	RubricVersion      string  `json:"rubric_version,omitempty"`
	Overall            string  `json:"overall"`
	EvaluatedAt        string  `json:"evaluated_at"`
	SupersededAt       *string `json:"superseded_at"`
}

// Get handles GET /runs/{id}/evaluation, optionally ?revision=.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	ev, err := h.Svc.Current(r.Context(), ws.ID, runID)
	if revision := r.URL.Query().Get("revision"); revision != "" {
		var revisionID pgtype.UUID
		if revisionID.Scan(revision) != nil {
			// A malformed id is 404 like a missing one: the client learns nothing
			// about which revisions exist either way.
			httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
		ev, err = h.Svc.Revision(r.Context(), ws.ID, runID, revisionID)
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evaluation lookup failed")
		return
	}

	view, err := h.Svc.view(r.Context(), ws.ID, ev)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evaluation lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

// Revisions handles GET /runs/{id}/evaluation/revisions, newest first.
func (h *Handler) Revisions(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	rows, err := h.Svc.Revisions(r.Context(), ws.ID, runID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evaluation lookup failed")
		return
	}
	out := make([]revisionView, 0, len(rows))
	for _, ev := range rows {
		out = append(out, revisionView{
			EvaluationID:       pgconv.UUIDString(ev.ID),
			JudgePromptVersion: derefString(ev.JudgePromptVersion),
			RubricVersion:      derefString(ev.RubricVersion),
			Overall:            ev.Overall,
			EvaluatedAt:        pgconv.RFC3339(ev.EvaluatedAt),
			SupersededAt:       optionalTime(ev.SupersededAt),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Revisions []revisionView `json:"revisions"`
	}{out})
}

// SetFeedback handles PUT /runs/{id}/evaluation/feedback (EVAL-001 clause 4).
//
// PUT and not POST: a user is allowed to change their mind, so sending it again
// replaces the previous answer instead of recording a second one. It attaches to
// the current revision — carrying an opinion about one verdict onto a later
// re-evaluation would be attributing something nobody said.
func (h *Handler) SetFeedback(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	var body struct {
		Helpful *bool  `json:"helpful"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a boolean `helpful`")
		return
	}
	if body.Helpful == nil {
		httpx.WriteError(w, http.StatusBadRequest, "`helpful` is required")
		return
	}
	if len([]rune(body.Comment)) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "`comment` is limited to 2000 characters")
		return
	}

	ev, err := h.Svc.SetFeedback(r.Context(), ws.ID, runID, *body.Helpful, body.Comment)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "feedback could not be recorded")
		return
	}
	view, err := h.Svc.view(r.Context(), ws.ID, ev)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evaluation lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

// --- service reads -----------------------------------------------------------

// Current returns the standing verdict for one run.
func (s *Service) Current(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Evaluation, error) {
	ev, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Evaluation{}, ErrNotFound
	}
	return ev, err
}

// Revision returns one particular judgement of one run.
func (s *Service) Revision(ctx context.Context, workspaceID, runID, id pgtype.UUID) (gen.Evaluation, error) {
	ev, err := s.queries().GetEvaluationRevision(ctx, gen.GetEvaluationRevisionParams{
		ID: id, RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Evaluation{}, ErrNotFound
	}
	return ev, err
}

// Revisions returns every judgement recorded for one run, newest first. A run
// that was never evaluated is ErrNotFound, for the same reason Current is.
func (s *Service) Revisions(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.Evaluation, error) {
	rows, err := s.queries().ListEvaluationRevisions(ctx, gen.ListEvaluationRevisionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows, nil
}

// SetFeedback records the user's answer on the current revision.
func (s *Service) SetFeedback(
	ctx context.Context, workspaceID, runID pgtype.UUID, helpful bool, comment string,
) (gen.Evaluation, error) {
	current, err := s.Current(ctx, workspaceID, runID)
	if err != nil {
		return gen.Evaluation{}, err
	}
	// An empty comment clears a previous one rather than storing "".
	var commentPtr *string
	if comment != "" {
		commentPtr = &comment
	}
	return s.queries().SetEvaluationFeedback(ctx, gen.SetEvaluationFeedbackParams{
		ID: current.ID, WorkspaceID: workspaceID,
		FeedbackHelpful: &helpful, FeedbackComment: commentPtr,
	})
}

// view renders one stored evaluation, re-answering `available` on every evidence
// reference as it goes.
//
// Availability is not read back from what was stored: trace_events is dropped by
// partition, so a ref written as available would go on claiming the original is
// still there long after retention removed it. One query per report answers it for
// all of them (ADR-026 decision 2).
func (s *Service) view(ctx context.Context, workspaceID pgtype.UUID, ev gen.Evaluation) (evaluationView, error) {
	var results []CriterionResult
	if len(ev.CriterionResults) > 0 {
		if err := json.Unmarshal(ev.CriterionResults, &results); err != nil {
			return evaluationView{}, err
		}
	}
	var findings []Finding
	if len(ev.DeterministicFindings) > 0 {
		if err := json.Unmarshal(ev.DeterministicFindings, &findings); err != nil {
			return evaluationView{}, err
		}
	}

	live, err := s.liveTraceEvents(ctx, workspaceID, ev.RunID, results, findings)
	if err != nil {
		return evaluationView{}, err
	}
	for i := range results {
		markAvailability(results[i].Evidence, live)
	}
	for i := range findings {
		markAvailability(findings[i].Evidence, live)
	}

	view := evaluationView{
		EvaluationID:          pgconv.UUIDString(ev.ID),
		RunID:                 pgconv.UUIDString(ev.RunID),
		Status:                ev.Status,
		Overall:               ev.Overall,
		Summary:               derefString(ev.Summary),
		CriterionResults:      orEmptyResults(results),
		DeterministicFindings: nonNilFindings(findings),
		JudgeModel:            derefString(ev.JudgeModel),
		JudgePromptVersion:    derefString(ev.JudgePromptVersion),
		RubricVersion:         derefString(ev.RubricVersion),
		EvidenceComplete:      ev.EvidenceComplete,
		Cost:                  costViewOf(ev),
		EvaluatedAt:           pgconv.RFC3339(ev.EvaluatedAt),
		SupersededAt:          optionalTime(ev.SupersededAt),
	}
	if ev.FeedbackHelpful != nil {
		view.Feedback = &feedbackView{
			Helpful: *ev.FeedbackHelpful, Comment: derefString(ev.FeedbackComment),
			// The row has no separate feedback timestamp; updated_at is what moves
			// with the two feedback columns (0024's trigger keeps exactly those
			// three writable together), so it is when the answer was given.
			SubmittedAt: pgconv.RFC3339(ev.UpdatedAt),
		}
	}
	return view, nil
}

// liveTraceEvents is the set of cited trace events that still resolve.
func (s *Service) liveTraceEvents(
	ctx context.Context, workspaceID, runID pgtype.UUID,
	results []CriterionResult, findings []Finding,
) (map[string]bool, error) {
	ids := map[string]bool{}
	collect := func(refs []EvidenceRef) {
		for _, ref := range refs {
			if ref.Kind == KindTraceEvent && ref.TraceEventID != "" {
				ids[ref.TraceEventID] = false
			}
		}
	}
	for _, r := range results {
		collect(r.Evidence)
	}
	for _, f := range findings {
		collect(f.Evidence)
	}
	if len(ids) == 0 {
		return ids, nil
	}

	uuids := make([]pgtype.UUID, 0, len(ids))
	for id := range ids {
		var u pgtype.UUID
		if u.Scan(id) == nil {
			uuids = append(uuids, u)
		}
	}
	rows, err := s.Trace.LiveEvents(ctx, workspaceID, runID, uuids)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		ids[pgconv.UUIDString(u)] = true
	}
	return ids, nil
}

// markAvailability tells the reader whether each citation can still be followed.
// A stale one keeps its excerpt and is labelled: never blanked out, and never
// presented as though the original were still there (ADR-009).
func markAvailability(refs []EvidenceRef, live map[string]bool) {
	for i := range refs {
		if refs[i].Kind == KindTraceEvent && refs[i].TraceEventID != "" {
			refs[i].Available = live[refs[i].TraceEventID]
		}
	}
}

func costViewOf(ev gen.Evaluation) costView {
	v := costView{Source: "estimated"}
	if ev.CostSource != nil {
		v.Source = *ev.CostSource
	}
	if ev.CostUsd.Valid {
		if f, err := ev.CostUsd.Float64Value(); err == nil && f.Valid {
			v.EvaluationUSD = &f.Float64
		}
	}
	switch {
	case v.EvaluationUSD == nil:
		v.Note = "the judge call reported no spend, so this is unreported and not $0. " +
			"The authoritative figure is the gateway's per-key spend for this evaluation_id (ADR-017)"
	case ev.CostIsLowerBound:
		v.Note = "what this judgement cost, never added to the run's own cost: the two are " +
			"spent by different workloads under different keys. The authoritative figure " +
			"is the gateway's per-key spend (ADR-017)"
	}
	return v
}

func orEmptyResults(r []CriterionResult) []CriterionResult {
	if r == nil {
		return []CriterionResult{}
	}
	return r
}

func pathUUID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return id, false
	}
	return id, true
}

// optionalTime keeps null meaningful: `superseded_at: null` is what says a
// revision is the standing one, so it is a JSON null and never an absent field.
func optionalTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}
