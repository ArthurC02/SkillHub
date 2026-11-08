package reflection

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

const maxAdoptBodyBytes = 1024

type pairedView struct {
	Verdict string   `json:"verdict"`
	Mean    *float64 `json:"mean_difference"`
	Low     *float64 `json:"low"`
	High    *float64 `json:"high"`
}

type standingView struct {
	Candidate            int        `json:"candidate"`
	SkillVersionID       *string    `json:"skill_version_id"`
	BatchID              *string    `json:"batch_id"`
	SkippedSuggestionIDs []string   `json:"skipped_suggestion_ids"`
	Settled              bool       `json:"settled"`
	Kept                 bool       `json:"kept"`
	Dev                  pairedView `json:"dev"`
	HeldOut              pairedView `json:"held_out"`
}

type adoptionView struct {
	Candidate      int    `json:"candidate"`
	SkillVersionID string `json:"skill_version_id"`
	AdoptedAt      string `json:"adopted_at"`
}

type evaluationView struct {
	Candidates  []standingView `json:"candidates"`
	Recommended *int           `json:"recommended"`
	Adopted     *adoptionView  `json:"adopted"`
}

type unconfirmedPairView struct {
	SkillVersionID string `json:"skill_version_id"`
	TestCaseID     string `json:"test_case_id"`
}

func optionalID(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	text := pgconv.UUIDString(id)
	return &text
}

func viewOfPaired(p PairedVerdict) pairedView {
	return pairedView{Verdict: p.Verdict, Mean: p.Mean, Low: p.Low, High: p.High}
}

func viewOfStandings(s CandidateStandings) *evaluationView {
	if len(s.Candidates) == 0 {
		return nil
	}
	out := &evaluationView{Candidates: make([]standingView, len(s.Candidates))}
	for i, c := range s.Candidates {
		skipped := make([]string, len(c.Skipped))
		for j, id := range c.Skipped {
			skipped[j] = pgconv.UUIDString(id)
		}
		out.Candidates[i] = standingView{
			Candidate: c.Candidate, SkillVersionID: optionalID(c.VersionID), BatchID: optionalID(c.BatchID),
			SkippedSuggestionIDs: skipped, Settled: c.Settled, Kept: c.Kept,
			Dev: viewOfPaired(c.Dev), HeldOut: viewOfPaired(c.HeldOut),
		}
	}
	if s.Recommended >= 0 {
		out.Recommended = &s.Recommended
	}
	if s.Adopted != nil {
		out.Adopted = &adoptionView{
			Candidate: s.Adopted.Candidate, SkillVersionID: pgconv.UUIDString(s.Adopted.VersionID),
			AdoptedAt: s.Adopted.AdoptedAt.Time.UTC().Format(time.RFC3339),
		}
	}
	return out
}

func roundPath(w http.ResponseWriter, r *http.Request) (EvaluateAsk, bool) {
	ids, ok := batchPath(w, r)
	if !ok {
		return EvaluateAsk{}, false
	}
	reflectionID, ok := httpx.PathUUID(w, r, "reflectionId", eval.ErrNotFound.Error())
	if !ok {
		return EvaluateAsk{}, false
	}
	return EvaluateAsk{SkillID: ids[0], BatchID: ids[1], ReflectionID: reflectionID}, true
}

func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	user, ws, ok := identity.SessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	ask, ok := roundPath(w, r)
	if !ok {
		return
	}
	ask.Actor = user.ID
	err := h.Svc.EvaluateCandidates(r.Context(), ws, ask)
	if unconfirmed, is := errors.AsType[UnconfirmedPairsError](err); is {
		pairs := make([]unconfirmedPairView, len(unconfirmed.Pairs))
		for i, p := range unconfirmed.Pairs {
			pairs[i] = unconfirmedPairView{SkillVersionID: pgconv.UUIDString(p.VersionID), TestCaseID: pgconv.UUIDString(p.TestCaseID)}
		}
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": "這幾題在候選版本上要先確認試跑權限；候選批次還沒開。", "unconfirmed_pairs": pairs,
		})
		return
	}
	if writeCandidateError(w, err, "評估候選失敗，可以再試一次；已經建好的版本會沿用，不會重建。") {
		return
	}
	h.writeRound(w, r.Context(), ws.ID, ask)
}

func (h *Handler) Adopt(w http.ResponseWriter, r *http.Request) {
	user, ws, ok := identity.SessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	ask, ok := roundPath(w, r)
	if !ok {
		return
	}
	var body struct {
		Candidate *int `json:"candidate"`
	}
	if !httpx.DecodeJSON(w, r, maxAdoptBodyBytes, &body, "body must be JSON with `candidate`") {
		return
	}
	if body.Candidate == nil {
		httpx.WriteError(w, http.StatusBadRequest, "`candidate` is required")
		return
	}
	ask.Actor = user.ID
	_, err := h.Svc.Adopt(r.Context(), ws, ask, *body.Candidate)
	if writeCandidateError(w, err, "採用失敗，可以再試一次。") {
		return
	}
	h.writeRound(w, r.Context(), ws.ID, ask)
}

type candidateVersionView struct {
	VersionID    string `json:"version_id"`
	ReflectionID string `json:"reflection_id"`
	BatchID      string `json:"batch_id"`
	Round        int    `json:"round"`
	Candidate    int    `json:"candidate"`
}

func (h *Handler) CandidateVersions(w http.ResponseWriter, r *http.Request) {
	_, ws, ok := identity.SessionWorkspace(h.Identity, w, r)
	if !ok {
		return
	}
	out := []candidateVersionView{}
	var skillID pgtype.UUID
	if skillID.Scan(r.PathValue("id")) != nil || h.Svc.Eval == nil || h.Svc.Candidates.Versions == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"candidate_versions": out})
		return
	}
	ids, err := h.Svc.Candidates.Versions(r.Context(), ws.ID, skillID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "candidate version lookup failed")
		return
	}
	versions, err := h.Svc.Eval.CandidateVersionsOf(r.Context(), ws.ID, ids)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "candidate version lookup failed")
		return
	}
	for _, v := range versions {
		out = append(out, candidateVersionView{
			VersionID: pgconv.UUIDString(v.VersionID), ReflectionID: pgconv.UUIDString(v.ReflectionID),
			BatchID: pgconv.UUIDString(v.BatchID), Round: v.Round, Candidate: v.Candidate,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"candidate_versions": out})
}

func writeCandidateError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, eval.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, eval.ErrNotFound.Error())
	case errors.Is(err, ErrRoundCapUnset), errors.Is(err, eval.ErrRoundVersionSuperseded), errors.Is(err, eval.ErrAlreadyAdopted):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrRoundOverCap), errors.Is(err, ErrEvaluateCreditThreshold), errors.Is(err, eval.ErrCandidateUnavailable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrModelCallsHalted):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, fallback)
	}
	return true
}

func (h *Handler) writeRound(w http.ResponseWriter, ctx context.Context, workspaceID pgtype.UUID, ask EvaluateAsk) {
	round, err := h.Svc.roundOfBatch(ctx, workspaceID, ask)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "reflection lookup failed")
		return
	}
	out, err := h.view(ctx, workspaceID, round)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "reflection lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
