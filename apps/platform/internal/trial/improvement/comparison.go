package eval

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

const runCostAuthority = "模型閘道對這個 Run 的 per-key 實付（ADR-017）"

type comparisonView struct {
	Runs            []comparisonSide `json:"runs"`
	CriterionMatrix []criterionRow   `json:"criterion_matrix"`
	VersionDiffURL  string           `json:"version_diff_url,omitempty"`
}

type comparisonSide struct {
	RunID string `json:"run_id"`

	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	TestCaseID     string `json:"test_case_id,omitempty"`
	Status         string `json:"status"`

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

type runCostView struct {
	Credits             *int64 `json:"credits"`
	IsLowerBound        bool   `json:"is_lower_bound"`
	AuthoritativeSource string `json:"authoritative_source"`
}

type criterionRow struct {
	CriterionID string             `json:"criterion_id"`
	Text        string             `json:"text"`
	Results     []criterionOutcome `json:"results"`
}

type criterionOutcome struct {
	RunID string `json:"run_id"`

	Result *string `json:"result"`
	Source string  `json:"source,omitempty"`
}

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

		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "comparison failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

type sideDetail struct {
	runID    string
	skillID  pgtype.UUID
	criteria []testlab.Criterion
	results  map[string]CriterionResult
}

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

	if side.InputsAvailable, err = q.RunInputsStillAvailable(ctx, gen.RunInputsStillAvailableParams{
		SnapshotID: snapshot.ID, WorkspaceID: workspaceID,
	}); err != nil {
		return comparisonSide{}, sideDetail{}, err
	}

	summary, err := s.Trace.General(ctx, workspaceID, runID)
	if err != nil {
		return comparisonSide{}, sideDetail{}, err
	}
	side.FinalOutput = summary.FinalOutput
	if len(summary.Errors) > 0 {
		side.Errors = summary.Errors
	}
	if summary.Usage != nil && summary.Usage.CostUSD != nil && s.Credits != nil {
		if c, ok := s.Credits(*summary.Usage.CostUSD); ok {
			side.Cost.Credits = &c
		}
	}
	if run.StartedAt != nil && run.FinishedAt != nil {
		ms := run.FinishedAt.Sub(*run.StartedAt).Milliseconds()
		side.DurationMS = &ms
	}

	ev, err := s.Current(ctx, workspaceID, runID)
	switch {
	case errors.Is(err, ErrNotFound):

		return side, detail, nil
	case err != nil:
		return comparisonSide{}, sideDetail{}, err
	}
	side.Evaluation = &comparisonVerdict{
		EvaluationID: pgconv.UUIDString(ev.ID),
		Status:       ev.Status,
		Overall:      ev.Overall,
		Cost:         costViewOf(ev, s.Credits),
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
