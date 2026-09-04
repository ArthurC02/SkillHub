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

	sets := make([][]EvidenceRef, 0, len(results)+len(findings))
	for i := range results {
		sets = append(sets, results[i].Evidence)
	}
	for i := range findings {
		sets = append(sets, findings[i].Evidence)
	}
	live, err := s.resolveEvidence(ctx, workspaceID, ev.RunID, sets...)
	if err != nil {
		return evaluationView{}, err
	}
	for _, refs := range sets {
		markAvailability(refs, live)
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

// liveEvidence is the answer to 「can this citation still be followed」 for both
// kinds of reference, gathered once per report rather than once per reference.
type liveEvidence struct {
	traceEvents map[string]bool
	artifacts   map[string]bool
	// finalOutput answers the third kind. An `agent_output` citation quotes
	// m.summary.FinalOutput, which is itself folded out of trace events - so when
	// the trace goes, the thing that citation points at goes with it, and it was
	// the last of the three still shipping a frozen `true`.
	finalOutput bool
}

// resolveEvidence re-answers availability for every reference in sets.
//
// ADR-026 decision 2 and doc.go's invariant 9 both say `available` is answered at
// READ time, and the reason is that a reference outlives the evidence it points
// at: trace_events is dropped by partition, and WS-002 clause 3 lets a user delete
// a run output whenever they like. A value read back from the stored JSON is
// therefore a claim about the past presented as a claim about now.
//
// All three kinds, because only one of them was ever answered. Artifact references
// were written `Available: true` by deterministic.go and never revisited, so a
// report went on citing a file the user had deleted as though it were still there;
// `agent_output` references were written the same way by judge.go's outputRef.
//
// The artifact half asks execution's own injected read (ReadEvaluationInput,
// backed by ListReadableRunArtifacts) rather than a query of its own: run outputs
// are execution's rows, and eval reaching into them directly is what ADR-033 is
// about.
func (s *Service) resolveEvidence(
	ctx context.Context, workspaceID, runID pgtype.UUID, sets ...[]EvidenceRef,
) (liveEvidence, error) {
	live := liveEvidence{traceEvents: map[string]bool{}, artifacts: map[string]bool{}}
	var wantArtifacts, wantOutput bool
	for _, refs := range sets {
		for _, ref := range refs {
			switch {
			case ref.Kind == KindTraceEvent && ref.TraceEventID != "":
				live.traceEvents[ref.TraceEventID] = false
			case ref.Kind == KindArtifact && ref.ArtifactPath != "":
				wantArtifacts = true
			case ref.Kind == KindAgentOutput:
				wantOutput = true
			}
		}
	}

	if len(live.traceEvents) > 0 {
		uuids := make([]pgtype.UUID, 0, len(live.traceEvents))
		for id := range live.traceEvents {
			var u pgtype.UUID
			if u.Scan(id) == nil {
				uuids = append(uuids, u)
			}
		}
		rows, err := s.Trace.LiveEvents(ctx, workspaceID, runID, uuids)
		if err != nil {
			return liveEvidence{}, err
		}
		for _, u := range rows {
			live.traceEvents[pgconv.UUIDString(u)] = true
		}
	}

	if wantArtifacts {
		if s.ReadEvaluationInput == nil {
			// Refuse rather than fall back to the stored value. Both composition
			// roots inject this, and the fallback is the exact claim this function
			// exists to stop making.
			return liveEvidence{}, errRunReaderNotConfigured
		}
		// found == false is a run that is gone, which makes every output gone with
		// it; the empty set below is the right answer and not an error.
		input, _, err := s.ReadEvaluationInput(ctx, workspaceID, runID)
		if err != nil {
			return liveEvidence{}, err
		}
		for _, a := range input.Artifacts {
			live.artifacts[a.FileName] = true
		}
	}

	if wantOutput {
		// The same read the evaluation itself used to quote from (gather's
		// m.summary), asked again now. An empty final output means the events it
		// was folded from are gone.
		summary, err := s.Trace.General(ctx, workspaceID, runID)
		if err != nil {
			return liveEvidence{}, err
		}
		live.finalOutput = summary.FinalOutput != ""
	}
	return live, nil
}

// markAvailability tells the reader whether each citation can still be followed.
// A stale one keeps its excerpt and is labelled: never blanked out, and never
// presented as though the original were still there (ADR-009).
func markAvailability(refs []EvidenceRef, live liveEvidence) {
	for i := range refs {
		switch {
		case refs[i].Kind == KindTraceEvent && refs[i].TraceEventID != "":
			refs[i].Available = live.traceEvents[refs[i].TraceEventID]
		case refs[i].Kind == KindArtifact && refs[i].ArtifactPath != "":
			refs[i].Available = live.artifacts[refs[i].ArtifactPath]
		case refs[i].Kind == KindAgentOutput:
			refs[i].Available = live.finalOutput
		}
	}
}

func costViewOf(ev gen.Evaluation) costView {
	// "unreported", not "estimated". eval.go's costSource() writes a source only
	// when the gateway reported one, and judge.go says it in as many words: the
	// internal contract has no estimated source, and an unrecognised label is
	// unreported accounting rather than permission to relabel it as an estimate.
	// The default used to be "estimated", so every evaluation without a gateway
	// figure came out of here as {"evaluation_usd": null, "source": "estimated"}
	// - a field claiming the platform estimated something next to a note saying
	// it has no figure at all (NFR-001: the UI must not mislead).
	v := costView{Source: "unreported"}
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
		v.Note = "Judge 這一次呼叫沒有回報花費：這裡是未測量，不是 0 美元。" +
			"權威數字是閘道對這個 evaluation_id 的 per-key 實付（ADR-017）。"
	case ev.CostIsLowerBound:
		v.Note = "權威數字是閘道對這個 evaluation_id 的 per-key 實付（ADR-017）。"
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
