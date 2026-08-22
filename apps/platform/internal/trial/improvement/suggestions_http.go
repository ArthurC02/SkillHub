package eval

// The improvement-suggestion surface of contracts/openapi/public.yaml (EVAL-002):
//
//	GET  /runs/{id}/suggestions                  what the current evaluation proposed
//	PUT  /suggestions/{id}/decision              accept or reject one, applying nothing
//	GET  /suggestions/{id}/diff                  what applying it would change
//	POST /skills/{id}/versions/from-suggestions  the accepted ones as ONE new version
//
// All four are workspace scoped from the session (iron rule 3) and answer 404 to
// anyone the material does not belong to — existence is itself private (WS-006).
//
// Deciding and applying are separate endpoints on purpose: accepting five
// suggestions has to produce one new version, not five intermediate ones nobody
// ever ran (evaluation-design §5.3).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// AppliedSuggestion is the eval-owned provenance view packaging may publish.
type AppliedSuggestion struct {
	EvaluationID pgtype.UUID
	Category     string
	TargetPath   string
}

// AppliedSuggestions keeps the generated evaluation row and database handle inside its owner.
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

// suggestionView is public.yaml's ImprovementSuggestion.
//
// `proposed_content` is deliberately not in it, as the contract has it: the way to
// see a proposed change is GET /suggestions/{id}/diff, which shows it against what
// is actually in the package. A raw replacement body on its own reads as though it
// were already the file.
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

// Suggestions handles GET /runs/{id}/suggestions: the current evaluation's set.
// A run with no evaluation is 404 for the same reason GET /runs/{id}/evaluation
// is — 「未評估」 is a state of its own and an empty body is what a UI renders as
// "nothing to improve".
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
	for _, row := range rows {
		out = append(out, toSuggestionView(row))
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		EvaluationID string           `json:"evaluation_id"`
		Suggestions  []suggestionView `json:"suggestions"`
	}{pgconv.UUIDString(ev.ID), out})
}

// Decide handles PUT /suggestions/{id}/decision (EVAL-002 clause 3).
//
// It changes no package. Repeatable: a later call replaces the decision — except
// on a suggestion already built into a version, because that version is history
// and the way back from it is another version (iron rule 4).
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
	// `pending` is the state a suggestion starts in; there is no request that means
	// "un-decide", so it is not settable.
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
	// 0024 refuses an applied suggestion that is not accepted, and it is right to:
	// the version was built from this. 409 rather than a database error, because the
	// request is well formed and the conflict is with something that already
	// happened.
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

// Diff handles GET /suggestions/{id}/diff (EVAL-002 clause 3): the change is
// viewable before it is applied. Served rather than assembled in the client
// because computing it needs the stored package bytes, and a client-side guess
// could disagree with what applying actually does.
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

// applyResponse is public.yaml's UploadResult plus the two fields the contract
// adds to it (`allOf`). Embedded, so the version half is rendered by the same code
// every other creation path uses.
type applyResponse struct {
	ingest.UploadResult
	AppliedSuggestionIDs []string  `json:"applied_suggestion_ids"`
	RejectedSuggestions  []Blocked `json:"rejected_suggestions"`
}

// ApplySuggestions handles POST /skills/{id}/versions/from-suggestions
// (EVAL-002 clause 4): the accepted suggestions become exactly one new version,
// and the version they were written against is not touched (iron rule 4).
//
// Creating a version is not permission to run it: the new content hashes
// differently, so the preflight summary changes and TEST-009 requires a fresh
// confirmation before anything runs.
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
	// Nothing is built from a suggestion nobody accepted, and the whole call is
	// refused rather than partly honoured: a 201 listing fewer ids than were asked
	// for reads as "these were applied and those were rejected", which is not what
	// happened here (EVAL-002 clause 3 — the decision is the user's).
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
		// Nothing was applied, so no version exists. The body says why per
		// suggestion; a 201 with an empty applied list would report a change that
		// never happened.
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
