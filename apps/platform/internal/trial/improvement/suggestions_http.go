package eval

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

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

type suggestionListResponse struct {
	EvaluationID string           `json:"evaluation_id"`
	Suggestions  []suggestionView `json:"suggestions"`
}

func toSuggestionView(row Suggestion) suggestionView {
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
	rows, err := h.Svc.Suggestions(r.Context(), ws.ID, ev.ID)
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
	httpx.WriteJSON(w, http.StatusOK, suggestionListResponse{
		EvaluationID: pgconv.UUIDString(ev.ID), Suggestions: out,
	})
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
		Decision Decision `json:"decision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a `decision`")
		return
	}

	if !body.Decision.chosen() {
		httpx.WriteError(w, http.StatusBadRequest, errNotAChoice.Error())
		return
	}

	row, err := h.Svc.Decide(r.Context(), ws.ID, id, body.Decision)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "suggestion not found")
	case errors.Is(err, errAcceptanceIsFinal):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "decision could not be recorded")
	default:
		httpx.WriteJSON(w, http.StatusOK, toSuggestionView(row))
	}
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

type rejectedSuggestionsResponse struct {
	Error               string    `json:"error"`
	RejectedSuggestions []Blocked `json:"rejected_suggestions"`
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
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, rejectedSuggestionsResponse{
			Error: message, RejectedSuggestions: rejected,
		})
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
