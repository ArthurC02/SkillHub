package eval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

type AppliedSuggestion struct {
	EvaluationID pgtype.UUID
	Category     string
	TargetPath   string
}

func (s *Service) AppliedSuggestions(ctx context.Context, versionID, workspaceID pgtype.UUID) ([]AppliedSuggestion, error) {
	rows, err := gen.New(s.Pool).ListSuggestionsAppliedToVersion(ctx, gen.ListSuggestionsAppliedToVersionParams{
		AppliedSkillVersionID: versionID,
		WorkspaceID:           workspaceID,
	})
	if err != nil {
		return nil, err
	}
	result := make([]AppliedSuggestion, len(rows))
	for i, row := range rows {
		result[i] = AppliedSuggestion{
			EvaluationID: row.EvaluationID,
			Category:     row.Category,
			TargetPath:   row.TargetPath,
		}
	}
	return result, nil
}

type suggestionView struct {
	SuggestionID          string        `json:"suggestion_id"`
	Category              string        `json:"category"`
	Problem               string        `json:"problem"`
	Evidence              []EvidenceRef `json:"evidence"`
	TargetPath            string        `json:"target_path"`
	ExpectedImpact        string        `json:"expected_impact"`
	Decision              string        `json:"decision"`
	DecidedAt             string        `json:"decided_at,omitempty"`
	AppliedSkillVersionID string        `json:"applied_skill_version_id,omitempty"`
}

func toSuggestionView(row gen.EvaluationSuggestion) suggestionView {
	var evidence []EvidenceRef
	if len(row.Evidence) > 0 {
		_ = json.Unmarshal(row.Evidence, &evidence)
	}
	if evidence == nil {
		evidence = []EvidenceRef{}
	}
	out := suggestionView{
		SuggestionID:   pgconv.UUIDString(row.ID),
		Category:       row.Category,
		Problem:        row.Problem,
		Evidence:       evidence,
		TargetPath:     row.TargetPath,
		ExpectedImpact: row.ExpectedImpact,
		Decision:       row.Decision,
	}
	if row.DecidedAt.Valid {
		out.DecidedAt = row.DecidedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.AppliedSkillVersionID.Valid {
		out.AppliedSkillVersionID = pgconv.UUIDString(row.AppliedSkillVersionID)
	}
	return out
}

func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	ev, err := h.Svc.Current(r.Context(), ws.ID, runID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evaluation lookup failed")
		return
	}
	rows, err := h.Svc.queries().ListEvaluationSuggestions(r.Context(),
		gen.ListEvaluationSuggestionsParams{EvaluationID: ev.ID, WorkspaceID: ws.ID})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "suggestion lookup failed")
		return
	}
	out := make([]suggestionView, 0, len(rows))
	sets := make([][]EvidenceRef, 0, len(rows))
	for _, row := range rows {
		v := toSuggestionView(row)
		out = append(out, v)
		sets = append(sets, v.Evidence)
	}

	live, err := h.Svc.resolveEvidence(r.Context(), ws.ID, runID, sets...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "evidence availability lookup failed")
		return
	}
	for _, refs := range sets {
		markAvailability(refs, live)
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		EvaluationID string           `json:"evaluation_id"`
		Suggestions  []suggestionView `json:"suggestions"`
	}{pgconv.UUIDString(ev.ID), out})
}

func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}

	var body struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a `decision`")
		return
	}

	if body.Decision != DecisionAccepted && body.Decision != DecisionRejected {
		httpx.WriteError(w, http.StatusBadRequest,
			"`decision` must be \"accepted\" or \"rejected\"")
		return
	}

	q := h.Svc.queries()
	current, err := q.GetEvaluationSuggestion(r.Context(), gen.GetEvaluationSuggestionParams{
		ID: id, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "suggestion not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "suggestion lookup failed")
		return
	}

	if current.AppliedSkillVersionID.Valid && body.Decision != DecisionAccepted {
		httpx.WriteError(w, http.StatusConflict,
			"this suggestion has already been built into a skill version, so its acceptance "+
				"cannot be withdrawn; create a further version to change the package again")
		return
	}

	row, err := q.DecideSuggestion(r.Context(), gen.DecideSuggestionParams{
		Decision: body.Decision, ID: id, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "suggestion not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "decision could not be recorded")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSuggestionView(row))
}

func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}

	diff, err := h.Svc.SuggestionDiff(r.Context(), ws.ID, id)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "suggestion not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "diff could not be computed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, diff)
}

type applyResponse struct {
	ingest.UploadResult
	AppliedSuggestionIDs []string  `json:"applied_suggestion_ids"`
	RejectedSuggestions  []Blocked `json:"rejected_suggestions"`
}

func (h *Handler) ApplySuggestions(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	var body struct {
		EvaluationID  string   `json:"evaluation_id"`
		SuggestionIDs []string `json:"suggestion_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest,
			"body must be JSON with `evaluation_id` and `suggestion_ids`")
		return
	}
	var evaluationID pgtype.UUID
	if err := evaluationID.Scan(body.EvaluationID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "`evaluation_id` must be a UUID")
		return
	}
	if len(body.SuggestionIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "`suggestion_ids` must name at least one suggestion")
		return
	}
	ids := make([]pgtype.UUID, 0, len(body.SuggestionIDs))
	for _, raw := range body.SuggestionIDs {
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "`suggestion_ids` must all be UUIDs")
			return
		}
		ids = append(ids, id)
	}

	res, err := h.Svc.ApplySuggestions(r.Context(), ws, skillID, evaluationID, ids)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "skill, evaluation or suggestion not found")
		return
	}

	if errors.Is(err, ErrNotAccepted) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error()+
			"; not accepted: "+join(res.NotAccepted))
		return
	}

	if errors.Is(err, errProvenanceNotRecorded) {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "the new version could not be created")
		return
	}

	rejected := res.Rejected
	if rejected == nil {
		rejected = []Blocked{}
	}
	if !res.Created {

		const message = "not one of the suggestions could be applied, so no version was created"
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, struct {
			Error               string    `json:"error"`
			RejectedSuggestions []Blocked `json:"rejected_suggestions"`
		}{message, rejected})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, applyResponse{
		UploadResult:         res.Version,
		AppliedSuggestionIDs: res.Applied,
		RejectedSuggestions:  rejected,
	})
}

func join(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
