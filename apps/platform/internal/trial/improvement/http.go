package eval

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

type costView struct {
	EvaluationCredits *int64 `json:"evaluation_credits"`
	Source            string `json:"source"`
	Note              string `json:"note"`
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

func (s *Service) Current(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Evaluation, error) {
	ev, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Evaluation{}, ErrNotFound
	}
	return ev, err
}

func (s *Service) Revision(ctx context.Context, workspaceID, runID, id pgtype.UUID) (gen.Evaluation, error) {
	ev, err := s.queries().GetEvaluationRevision(ctx, gen.GetEvaluationRevisionParams{
		ID: id, RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Evaluation{}, ErrNotFound
	}
	return ev, err
}

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

func (s *Service) SetFeedback(
	ctx context.Context, workspaceID, runID pgtype.UUID, helpful bool, comment string,
) (gen.Evaluation, error) {
	current, err := s.Current(ctx, workspaceID, runID)
	if err != nil {
		return gen.Evaluation{}, err
	}

	var commentPtr *string
	if comment != "" {
		commentPtr = &comment
	}
	return s.queries().SetEvaluationFeedback(ctx, gen.SetEvaluationFeedbackParams{
		ID: current.ID, WorkspaceID: workspaceID,
		FeedbackHelpful: &helpful, FeedbackComment: commentPtr,
	})
}

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
		Cost:                  costViewOf(ev, s.Credits),
		EvaluatedAt:           pgconv.RFC3339(ev.EvaluatedAt),
		SupersededAt:          optionalTime(ev.SupersededAt),
	}
	if ev.FeedbackHelpful != nil {
		view.Feedback = &feedbackView{
			Helpful: *ev.FeedbackHelpful, Comment: derefString(ev.FeedbackComment),

			SubmittedAt: pgconv.RFC3339(ev.UpdatedAt),
		}
	}
	return view, nil
}

type liveEvidence struct {
	traceEvents map[string]bool
	artifacts   map[string]bool

	finalOutput bool
}

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

			return liveEvidence{}, errRunReaderNotConfigured
		}

		input, _, err := s.ReadEvaluationInput(ctx, workspaceID, runID)
		if err != nil {
			return liveEvidence{}, err
		}
		for _, a := range input.Artifacts {
			live.artifacts[a.FileName] = true
		}
	}

	if wantOutput {

		summary, err := s.Trace.General(ctx, workspaceID, runID)
		if err != nil {
			return liveEvidence{}, err
		}
		live.finalOutput = summary.FinalOutput != ""
	}
	return live, nil
}

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

func costViewOf(ev gen.Evaluation, credits func(float64) (int64, bool)) costView {

	v := costView{Source: "unreported"}
	if ev.CostSource != nil {
		v.Source = *ev.CostSource
	}

	if ev.CostUsd.Valid && credits != nil {
		if f, err := ev.CostUsd.Float64Value(); err == nil && f.Valid {
			if c, ok := credits(f.Float64); ok {
				v.EvaluationCredits = &c
			}
		}
	}
	switch {
	case v.EvaluationCredits == nil:
		v.Note = "Judge 這一次呼叫沒有回報花費：這裡是未測量，不是 0 點。" +
			"權威數字是閘道對這個 evaluation_id 的 per-key 實付（ADR-017）換算的點數。"
	case ev.CostIsLowerBound:
		v.Note = "權威數字是閘道對這個 evaluation_id 的 per-key 實付（ADR-017）換算的點數。"
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

func optionalTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}
