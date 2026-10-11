package eval

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const maxBatchBodyBytes = 8 << 10

type batchView struct {
	ID             string      `json:"id"`
	SkillID        string      `json:"skill_id"`
	SkillVersionID string      `json:"skill_version_id"`
	Trials         int         `json:"trials"`
	Status         BatchStatus `json:"status"`
	StopReason     string      `json:"stop_reason,omitempty"`
	CreatedAt      string      `json:"created_at"`
}

type batchTrialView struct {
	Trial    int          `json:"trial"`
	RunID    string       `json:"run_id,omitempty"`
	EndRunID string       `json:"end_run_id,omitempty"`
	Outcome  TrialOutcome `json:"outcome"`
}

type batchTestCaseView struct {
	TestCaseID         string           `json:"test_case_id"`
	Trials             []batchTrialView `json:"trials"`
	Passed             int              `json:"passed"`
	Judged             int              `json:"judged"`
	PassAtK            *bool            `json:"pass_at_k,omitempty"`
	PassAllK           *bool            `json:"pass_all_k,omitempty"`
	PromptVariedAcross bool             `json:"prompt_varied_across"`
}

type intervalView struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

type batchReportView struct {
	Batch              batchView           `json:"batch"`
	TestCases          []batchTestCaseView `json:"test_cases"`
	Passed             int                 `json:"passed"`
	Judged             int                 `json:"judged"`
	PassRate           *float64            `json:"pass_rate,omitempty"`
	PassRate95         *intervalView       `json:"pass_rate_95,omitempty"`
	TestCasesAllPassed int                 `json:"test_cases_all_passed"`
	TestCasesDecided   int                 `json:"test_cases_decided"`
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.sessionWorkspace(w, r)
	if !ok {
		return
	}
	skillID, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body struct {
		SkillVersionID string   `json:"skill_version_id"`
		TestCaseIDs    []string `json:"test_case_ids"`
		Trials         int      `json:"trials"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with `skill_version_id`, `test_case_ids` and `trials`")
		return
	}
	ids, ok := uuids(append([]string{body.SkillVersionID}, body.TestCaseIDs...))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "`skill_version_id` and `test_case_ids` must be UUIDs")
		return
	}
	batch, err := h.Svc.CreateBatch(r.Context(), NewBatch{
		WorkspaceID: ws.ID, Actor: user, SkillID: skillID,
		VersionID: ids[0], TestCaseIDs: ids[1:], Trials: body.Trials,
	})
	if unconfirmed, isUnconfirmed := errors.AsType[UnconfirmedTestCasesError](err); isUnconfirmed {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": err.Error(), "unconfirmed_test_case_ids": uuidStrings(unconfirmed.TestCaseIDs),
		})
		return
	}
	switch {
	case errors.Is(err, ErrBatchShape):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "skill, version or test case not found")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "the batch could not be created")
	default:
		httpx.WriteJSON(w, http.StatusCreated, viewOfBatch(batch))
	}
}

func (h *Handler) ListBatches(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := pathUUID(w, r)
	if !ok {
		return
	}
	batches, err := h.Svc.ListBatches(r.Context(), ws.ID, skillID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "batch lookup failed")
		return
	}
	views := make([]batchView, len(batches))
	for i, b := range batches {
		views[i] = viewOfBatch(b)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"batches": views})
}

func (h *Handler) BatchReport(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var batchID pgtype.UUID
	if err := batchID.Scan(r.PathValue("batchId")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	report, err := h.Svc.BatchReport(r.Context(), ws.ID, skillID, batchID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "batch report failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewOfReport(report))
}

type batchComparisonView struct {
	Baseline        batchView     `json:"baseline"`
	Candidate       batchView     `json:"candidate"`
	Paired          int           `json:"paired"`
	Unpaired        int           `json:"unpaired"`
	Difference      *float64      `json:"difference,omitempty"`
	Difference95    *intervalView `json:"difference_95,omitempty"`
	CandidateBetter int           `json:"candidate_better"`
	BaselineBetter  int           `json:"baseline_better"`
	Tied            int           `json:"tied"`
}

func (h *Handler) CompareBatches(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	ids, ok := uuids([]string{r.PathValue("id"), r.PathValue("batchId"), r.URL.Query().Get("against")})
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	comparison, err := h.Svc.CompareBatches(r.Context(), ws.ID, ids[0], ids[1], ids[2])
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "batch comparison failed")
		return
	}
	view := batchComparisonView{
		Baseline: viewOfBatch(comparison.Baseline), Candidate: viewOfBatch(comparison.Candidate),
		Paired: comparison.Paired, Unpaired: comparison.Unpaired, Difference: comparison.Difference,
		CandidateBetter: comparison.CandidateBetter, BaselineBetter: comparison.BaselineBetter, Tied: comparison.Tied,
	}
	if comparison.Difference95 != nil {
		view.Difference95 = &intervalView{Low: comparison.Difference95.Low, High: comparison.Difference95.High}
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *Handler) sessionWorkspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, pgtype.UUID, bool) {
	user, _ := identity.SessionUser(r.Context())
	ws, ok := h.workspace(w, r)
	return ws, user.ID, ok
}

func uuids(raw []string) ([]pgtype.UUID, bool) {
	out := make([]pgtype.UUID, len(raw))
	for i, s := range raw {
		if err := out[i].Scan(s); err != nil {
			return nil, false
		}
	}
	return out, true
}

func uuidStrings(ids []pgtype.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = pgconv.UUIDString(id)
	}
	return out
}

func optionalUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return pgconv.UUIDString(id)
}

func viewOfBatch(b Batch) batchView {
	return batchView{
		ID: pgconv.UUIDString(b.ID), SkillID: pgconv.UUIDString(b.SkillID), SkillVersionID: pgconv.UUIDString(b.VersionID),
		Trials: b.Trials, Status: b.Status, StopReason: b.StopReason, CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func viewOfReport(report BatchReport) batchReportView {
	view := batchReportView{
		Batch: viewOfBatch(report.Batch), TestCases: make([]batchTestCaseView, len(report.TestCases)),
		Passed: report.Passed, Judged: report.Judged, PassRate: report.PassRate,
		TestCasesAllPassed: report.TestCasesAllK, TestCasesDecided: report.TestCasesDecided,
	}
	if report.PassRate95 != nil {
		view.PassRate95 = &intervalView{Low: report.PassRate95.Low, High: report.PassRate95.High}
	}
	for i, tc := range report.TestCases {
		trials := make([]batchTrialView, len(tc.Trials))
		for j, t := range tc.Trials {
			trials[j] = batchTrialView{
				Trial: t.Trial, RunID: optionalUUIDString(t.RunID), EndRunID: optionalUUIDString(t.EndRunID), Outcome: t.Outcome,
			}
		}
		view.TestCases[i] = batchTestCaseView{
			TestCaseID: pgconv.UUIDString(tc.TestCaseID), Trials: trials, Passed: tc.Passed, Judged: tc.Judged,
			PassAtK: tc.PassAtK, PassAllK: tc.PassAllK, PromptVariedAcross: tc.PromptVariedAcross,
		}
	}
	return view
}
