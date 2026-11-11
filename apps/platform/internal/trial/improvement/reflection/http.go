package reflection

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

const maxReflectBodyBytes = 16384

type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

type view struct {
	ReflectionID   string                `json:"reflection_id"`
	BatchID        string                `json:"batch_id"`
	DevTestCaseIDs []string              `json:"dev_test_case_ids"`
	Outcome        string                `json:"outcome"`
	RunsRead       int32                 `json:"runs_read"`
	RunsNotRead    int32                 `json:"runs_not_read"`
	CreatedAt      string                `json:"created_at"`
	Suggestions    []eval.SuggestionView `json:"suggestions"`
	Candidates     []candidateView       `json:"candidates"`
	Evaluation     *evaluationView       `json:"evaluation"`
}

type candidateView struct {
	Focus         string   `json:"focus"`
	Outcome       string   `json:"outcome"`
	SuggestionIDs []string `json:"suggestion_ids"`
}

func candidatesOf(rec eval.ReflectionRecord) []candidateView {
	out := make([]candidateView, len(rec.Row.CandidateFocuses))
	for i, focus := range rec.Row.CandidateFocuses {
		out[i] = candidateView{Focus: focus, Outcome: rec.Row.CandidateOutcomes[i], SuggestionIDs: []string{}}
	}
	for _, sug := range rec.Suggestions {
		if int(sug.Candidate) < len(out) {
			c := &out[sug.Candidate]
			c.SuggestionIDs = append(c.SuggestionIDs, pgconv.UUIDString(sug.ID))
		}
	}
	return out
}

func (h *Handler) Reflect(w http.ResponseWriter, r *http.Request) {
	user, ws, ok := identity.SessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	ids, ok := batchPath(w, r)
	if !ok {
		return
	}
	var body struct {
		DevTestCaseIDs []string `json:"dev_test_case_ids"`
	}
	if !httpx.DecodeJSON(w, r, maxReflectBodyBytes, &body, "body must be JSON with `dev_test_case_ids`") {
		return
	}
	devIDs := make([]pgtype.UUID, 0, len(body.DevTestCaseIDs))
	for _, raw := range body.DevTestCaseIDs {
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "`dev_test_case_ids` must all be UUIDs")
			return
		}
		devIDs = append(devIDs, id)
	}
	rec, err := h.Svc.Reflect(r.Context(), ws, Ask{
		Actor: user.ID, SkillID: ids[0], BatchID: ids[1], DevTestCaseIDs: devIDs,
	})
	switch {
	case errors.Is(err, eval.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, eval.ErrNotFound.Error())
	case errors.Is(err, ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotReady), errors.Is(err, ErrRoundCapUnset), errors.Is(err, eval.ErrRoundVersionSuperseded):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCreditThreshold), errors.Is(err, ErrRoundOverCap):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrModelCallsHalted), errors.Is(err, ErrBudgetExhausted):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "找改進失敗，這次沒有留下紀錄。")
	default:
		out, err := h.view(r.Context(), ws.ID, rec)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "evidence availability lookup failed")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	_, ws, ok := identity.SessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	ids, ok := batchPath(w, r)
	if !ok {
		return
	}
	recs, err := h.Svc.List(r.Context(), ws.ID, ids[0], ids[1])
	if errors.Is(err, eval.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, eval.ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "reflection lookup failed")
		return
	}
	out := make([]view, 0, len(recs))
	for _, rec := range recs {
		v, err := h.view(r.Context(), ws.ID, rec)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "evidence availability lookup failed")
			return
		}
		out = append(out, v)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reflections": out})
}

func batchPath(w http.ResponseWriter, r *http.Request) ([2]pgtype.UUID, bool) {
	var ids [2]pgtype.UUID
	for i, raw := range []string{r.PathValue("id"), r.PathValue("batchId")} {
		if err := ids[i].Scan(raw); err != nil {
			httpx.WriteError(w, http.StatusNotFound, eval.ErrNotFound.Error())
			return ids, false
		}
	}
	return ids, true
}

func (h *Handler) view(ctx context.Context, workspaceID pgtype.UUID, rec eval.ReflectionRecord) (view, error) {
	row := rec.Row
	suggestions, err := eval.ReflectionSuggestionViews(ctx, h.Svc.Eval, workspaceID, rec.Suggestions)
	if err != nil {
		return view{}, err
	}
	out := view{
		ReflectionID: pgconv.UUIDString(row.ID), BatchID: pgconv.UUIDString(row.BatchID),
		DevTestCaseIDs: make([]string, len(row.DevTestCaseIds)), Outcome: row.Outcome,
		RunsRead: row.RunsRead, RunsNotRead: row.RunsNotRead,
		CreatedAt:   pgconv.RFC3339(row.CreatedAt),
		Suggestions: suggestions,
		Candidates:  candidatesOf(rec),
	}
	standings, err := h.Svc.Standings(ctx, workspaceID, rec)
	if err != nil {
		return view{}, err
	}
	out.Evaluation = viewOfStandings(standings)
	for i, id := range row.DevTestCaseIds {
		out.DevTestCaseIDs[i] = pgconv.UUIDString(id)
	}
	return out, nil
}
