package eval

// GET /runs/{id}/comparison?against= — EVAL-012, 02:EVAL-003 clause 2.
//
// A read and nothing else. Both runs are frozen snapshots, so putting them side
// by side cannot rewrite either one (iron rule 4, 02:EVAL-003 clause 3); there is
// no write path in this file for the same reason there is no re-run endpoint.
//
// Re-running is the ordinary POST /skills/{id}/runs with the new
// `skill_version_id` and the same `test_case_id`, which keeps preflight and
// `confirmed_summary_hash` on the path (design §5.4). A one-click re-run here
// would be a second way to start a run, and the only thing it could skip is the
// permission screen TEST-009 exists to force.
//
// Two facts are reported per side and never merged: `status` is what happened
// while the run executed, `evaluation` is whether the task was achieved
// (ADR-025). A side with no evaluation has the field absent — 「未評估」, which is
// not a pass.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// runCostAuthority names where the settling figure lives, in the response rather
// than in a UI's memory (ADR-017, handoff 丙-3).
const runCostAuthority = "模型閘道對這個 Run 的 per-key 實付（ADR-017）"

type comparisonView struct {
	Runs            []comparisonSide `json:"runs"`
	CriterionMatrix []criterionRow   `json:"criterion_matrix"`
	VersionDiffURL  string           `json:"version_diff_url,omitempty"`
}

type comparisonSide struct {
	RunID string `json:"run_id"`
	// SkillID and TestCaseID are what a re-run would be started from, per side
	// because two runs of different skills may be compared. They are ids and not
	// permission: re-running is still POST /skills/{id}/runs behind preflight, and
	// InputsAvailable below is what says whether these inputs exist at all.
	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	TestCaseID     string `json:"test_case_id,omitempty"`
	Status         string `json:"status"`
	// Evaluation is absent when this run was never evaluated. Absent and not a
	// zero value: a rendered empty verdict is what a screen shows as a pass.
	Evaluation      *comparisonVerdict   `json:"evaluation,omitempty"`
	FinalOutput     string               `json:"final_output,omitempty"`
	Errors          []trace.ErrorSummary `json:"errors"`
	DurationMS      *int64               `json:"duration_ms,omitempty"`
	Cost            runCostView          `json:"cost"`
	InputsAvailable bool                 `json:"inputs_available"`
}

type comparisonVerdict struct {
	EvaluationID string   `json:"evaluation_id"`
	Status       string   `json:"status"`
	Overall      string   `json:"overall"`
	Cost         costView `json:"cost"`
}

// runCostView is what the *run* spent. Kept beside the evaluation's own cost and
// never added to it: one is the user's workload under the run's short-lived key,
// the other is the platform's verdict under the platform's key (handoff 丙-3).
//
// IsLowerBound is constant true and is not a flag anybody sets. The figure is
// summed from trace `usage` events and that sum is structurally incomplete — a
// response still in flight when the stream ends is not in it, and a producer's
// last flush can land after this was read.
type runCostView struct {
	USD                 *float64 `json:"usd"`
	IsLowerBound        bool     `json:"is_lower_bound"`
	AuthoritativeSource string   `json:"authoritative_source"`
}

type criterionRow struct {
	CriterionID string             `json:"criterion_id"`
	Text        string             `json:"text"`
	Results     []criterionOutcome `json:"results"`
}

type criterionOutcome struct {
	RunID string `json:"run_id"`
	// Result is null when this side has no verdict for the criterion: it was not
	// evaluated, or the criterion is not in that snapshot at all. Different from
	// `undetermined`, which is a verdict somebody reached.
	Result *string `json:"result"`
	Source string  `json:"source,omitempty"`
}

// Comparison handles GET /runs/{id}/comparison?against=.
func (h *Handler) Comparison(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r)
	if !ok {
		return
	}

	against := r.URL.Query().Get("against")
	if against == "" {
		httpx.WriteError(w, http.StatusBadRequest, "`against` is required: name the other run to compare with")
		return
	}
	var againstID pgtype.UUID
	if againstID.Scan(against) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "`against` must be a run UUID")
		return
	}
	if againstID == runID {
		httpx.WriteError(w, http.StatusBadRequest, "a run cannot be compared with itself")
		return
	}

	view, err := h.Svc.Comparison(r.Context(), ws.ID, runID, againstID)
	if errors.Is(err, ErrNotFound) {
		// Either run being someone else's is the same answer as it not existing
		// (WS-006), and the caller cannot tell which of the two it was.
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "comparison failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

// sideDetail is what the matrix needs from a side and the side itself does not
// carry: the criteria as the snapshot froze them, and the verdicts by id.
type sideDetail struct {
	runID    string
	skillID  pgtype.UUID
	criteria []testlab.Criterion
	results  map[string]CriterionResult
}

// Comparison reads both runs under one workspace (iron rule 3) and returns them
// side by side. Nothing is written.
func (s *Service) Comparison(
	ctx context.Context, workspaceID, runID, againstID pgtype.UUID,
) (comparisonView, error) {
	left, leftDetail, err := s.comparisonSide(ctx, workspaceID, runID)
	if err != nil {
		return comparisonView{}, err
	}
	right, rightDetail, err := s.comparisonSide(ctx, workspaceID, againstID)
	if err != nil {
		return comparisonView{}, err
	}

	view := comparisonView{
		Runs:            []comparisonSide{left, right},
		CriterionMatrix: criterionMatrix(leftDetail, rightDetail),
	}
	// The existing version diff (WS-003), not a second implementation of one. It
	// only exists between two versions of the same skill; comparing runs of
	// different skills is allowed and simply has no diff to link.
	if leftDetail.skillID == rightDetail.skillID && left.SkillVersionID != right.SkillVersionID {
		view.VersionDiffURL = "/skills/" + pgconv.UUIDString(leftDetail.skillID) + "/diff" +
			"?from=" + left.SkillVersionID + "&to=" + right.SkillVersionID
	}
	return view, nil
}

func (s *Service) comparisonSide(
	ctx context.Context, workspaceID, runID pgtype.UUID,
) (comparisonSide, sideDetail, error) {
	if s.ReadVersion == nil {
		return comparisonSide{}, sideDetail{}, errRegistryReadNotConfigured
	}
	q := s.queries()
	run, err := s.runFacts(ctx, workspaceID, runID)
	if err != nil {
		return comparisonSide{}, sideDetail{}, err
	}

	side := comparisonSide{
		RunID:          pgconv.UUIDString(run.ID),
		SkillVersionID: pgconv.UUIDString(run.SkillVersionID),
		Status:         run.Status,
		Errors:         []trace.ErrorSummary{},
		Cost:           runCostView{IsLowerBound: true, AuthoritativeSource: runCostAuthority},
	}
	detail := sideDetail{runID: side.RunID, results: map[string]CriterionResult{}}

	version, found, err := s.ReadVersion(ctx, workspaceID, run.SkillVersionID)
	if !found && err == nil {
		return comparisonSide{}, sideDetail{}, ErrNotFound
	}
	if err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	detail.skillID = version.SkillID
	side.SkillID = pgconv.UUIDString(version.SkillID)

	if err := s.requireTestLab(); err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	snapshot, err := s.TestLab.ReadSnapshot(ctx, workspaceID, run.TestCaseSnapshotID)
	if err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	side.TestCaseID = pgconv.UUIDString(snapshot.TestCaseID)
	if detail.criteria, err = testlab.DecodeCriteria(snapshot.AcceptanceCriteria); err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	// ADR-003 刪除與可追溯性: the snapshot outlives its inputs, so whether the run
	// could be repeated is a separate question from whether it can be read about.
	if side.InputsAvailable, err = q.RunInputsStillAvailable(ctx, gen.RunInputsStillAvailableParams{
		SnapshotID: snapshot.ID, WorkspaceID: workspaceID,
	}); err != nil {
		return comparisonSide{}, sideDetail{}, err
	}

	// Output, errors and the run's own cost come from trace.Service and from
	// nowhere else (handoff 丙-1): no query in this package reads trace_events
	// directly, so the completeness rules that view applies apply here too.
	summary, err := s.Trace.General(ctx, workspaceID, runID)
	if err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	side.FinalOutput = summary.FinalOutput
	if len(summary.Errors) > 0 {
		side.Errors = summary.Errors
	}
	if summary.Usage != nil {
		side.Cost.USD = summary.Usage.CostUSD
	}
	if run.StartedAt != nil && run.FinishedAt != nil {
		ms := run.FinishedAt.Sub(*run.StartedAt).Milliseconds()
		side.DurationMS = &ms
	}

	ev, err := s.Current(ctx, workspaceID, runID)
	switch {
	case errors.Is(err, ErrNotFound):
		// Never evaluated. The field stays absent (ADR-025 落地要求).
		return side, detail, nil
	case err != nil:
		return comparisonSide{}, sideDetail{}, err
	}
	side.Evaluation = &comparisonVerdict{
		EvaluationID: pgconv.UUIDString(ev.ID),
		Status:       ev.Status,
		Overall:      ev.Overall,
		Cost:         costViewOf(ev),
	}
	var results []CriterionResult
	if len(ev.CriterionResults) > 0 {
		if err := json.Unmarshal(ev.CriterionResults, &results); err != nil {
			return comparisonSide{}, sideDetail{}, err
		}
	}
	for _, r := range results {
		detail.results[r.CriterionID] = r
	}
	return side, detail, nil
}

// criterionMatrix is one row per acceptance criterion with both verdicts, so a
// regression shows up per criterion instead of only in the overall.
//
// Rows come from the snapshots and not from the verdicts: what was asked of a run
// is a fact about the run whether or not anything ever judged it, and building the
// matrix out of criterion_results would make an unevaluated side look like a run
// with no acceptance criteria. Criteria are matched by id, which the two snapshots
// share when the same test case was re-run.
func criterionMatrix(left, right sideDetail) []criterionRow {
	rows := make([]criterionRow, 0, len(left.criteria)+len(right.criteria))
	seen := map[string]bool{}
	add := func(criteria []testlab.Criterion) {
		for _, c := range criteria {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			rows = append(rows, criterionRow{
				CriterionID: c.ID,
				Text:        c.Text,
				Results: []criterionOutcome{
					outcomeOf(left, c.ID),
					outcomeOf(right, c.ID),
				},
			})
		}
	}
	// The run named in the path first, then whatever only the other side asked —
	// dropping those would hide exactly the criteria that were added or removed
	// between the two runs.
	add(left.criteria)
	add(right.criteria)
	return rows
}

func outcomeOf(side sideDetail, criterionID string) criterionOutcome {
	out := criterionOutcome{RunID: side.runID}
	if r, ok := side.results[criterionID]; ok {
		result := r.Result
		out.Result, out.Source = &result, r.Source
	}
	return out
}
